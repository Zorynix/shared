package logger

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	UserIDKey    ContextKey = "user_id"
	ServiceKey   ContextKey = "service"
	OperationKey ContextKey = "operation"
	TraceIDKey   ContextKey = "trace_id"
)

type Logger struct {
	*logrus.Logger
	serviceName string
}

type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
	LevelFatal LogLevel = "fatal"
)

type Config struct {
	Level       LogLevel `yaml:"level" validate:"required,oneof=debug info warn error fatal"`
	Format      string   `yaml:"format" validate:"required,oneof=json text"`
	ServiceName string   `yaml:"service_name" validate:"required"`
	Environment string   `yaml:"environment" validate:"required"`
	Output      string   `yaml:"output" validate:"oneof=stdout stderr file"`
	FilePath    string   `yaml:"file_path,omitempty"`
}

type Fields map[string]interface{}

func NewLogger(config Config) (*Logger, error) {
	logger := logrus.New()

	level, err := logrus.ParseLevel(string(config.Level))
	if err != nil {
		return nil, err
	}
	logger.SetLevel(level)

	switch config.Format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: time.RFC3339Nano,
			FullTimestamp:   true,
		})
	}

	switch config.Output {
	case "stderr":
		logger.SetOutput(os.Stderr)
	case "file":
		if config.FilePath == "" {
			return nil, logrus.NewEntry(logger).WithError(err).Logger
		}
		file, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		logger.SetOutput(file)
	default:
		logger.SetOutput(os.Stdout)
	}

	return &Logger{
		Logger:      logger,
		serviceName: config.ServiceName,
	}, nil
}

func (l *Logger) WithContext(ctx context.Context) *logrus.Entry {
	entry := l.Logger.WithFields(logrus.Fields{
		"service": l.serviceName,
	})

	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		entry = entry.WithField("request_id", requestID)
	}
	if userID := ctx.Value(UserIDKey); userID != nil {
		entry = entry.WithField("user_id", userID)
	}
	if operation := ctx.Value(OperationKey); operation != nil {
		entry = entry.WithField("operation", operation)
	}
	if traceID := ctx.Value(TraceIDKey); traceID != nil {
		entry = entry.WithField("trace_id", traceID)
	}

	return entry
}

func (l *Logger) WithFields(fields Fields) *logrus.Entry {
	logrusFields := make(logrus.Fields)
	for k, v := range fields {
		logrusFields[k] = v
	}
	logrusFields["service"] = l.serviceName
	return l.Logger.WithFields(logrusFields)
}

func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err).WithField("service", l.serviceName)
}

func (l *Logger) LogRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	l.WithContext(ctx).WithFields(logrus.Fields{
		"http_method":      method,
		"http_path":        path,
		"http_status":      statusCode,
		"response_time_ms": duration.Milliseconds(),
		"type":             "http_request",
	}).Info("HTTP request processed")
}

func (l *Logger) LogGRPCRequest(ctx context.Context, method string, duration time.Duration, err error) {
	fields := logrus.Fields{
		"grpc_method":      method,
		"response_time_ms": duration.Milliseconds(),
		"type":             "grpc_request",
	}

	entry := l.WithContext(ctx).WithFields(fields)
	if err != nil {
		entry.WithError(err).Error("gRPC request failed")
	} else {
		entry.Info("gRPC request processed")
	}
}

func (l *Logger) LogDatabaseQuery(ctx context.Context, query string, duration time.Duration, err error) {
	fields := logrus.Fields{
		"db_query":      query,
		"query_time_ms": duration.Milliseconds(),
		"type":          "database_query",
	}

	entry := l.WithContext(ctx).WithFields(fields)
	if err != nil {
		entry.WithError(err).Error("Database query failed")
	} else {
		entry.Debug("Database query executed")
	}
}

func (l *Logger) LogCacheOperation(ctx context.Context, operation, key string, hit bool, duration time.Duration) {
	l.WithContext(ctx).WithFields(logrus.Fields{
		"cache_operation":   operation,
		"cache_key":         key,
		"cache_hit":         hit,
		"operation_time_ms": duration.Milliseconds(),
		"type":              "cache_operation",
	}).Debug("Cache operation")
}

func (l *Logger) LogBusinessEvent(ctx context.Context, event string, data interface{}) {
	dataJSON, _ := json.Marshal(data)
	l.WithContext(ctx).WithFields(logrus.Fields{
		"business_event": event,
		"event_data":     string(dataJSON),
		"type":           "business_event",
	}).Info("Business event occurred")
}

func (l *Logger) LogSecurity(ctx context.Context, event, details string, severity string) {
	l.WithContext(ctx).WithFields(logrus.Fields{
		"security_event":   event,
		"security_details": details,
		"severity":         severity,
		"type":             "security_event",
	}).Warn("Security event")
}

func CreateContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

func CreateContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

func CreateContextWithOperation(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, OperationKey, operation)
}

func CreateContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}
