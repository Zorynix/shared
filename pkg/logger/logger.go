package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	EnvLocal       = "local"
	EnvDev         = "dev"
	EnvDevelopment = "development"
	EnvProd        = "prod"
	EnvProduction  = "production"
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
	*zap.Logger
	serviceName string
	environment string
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
	Output      string   `yaml:"output" validate:"required,oneof=stdout stderr file"`
	FilePath    string   `yaml:"file_path"`
}

type Fields map[string]interface{}

func New(env string) *Logger {
	var logger *zap.Logger

	switch env {
	case EnvLocal, EnvDev, EnvDevelopment:
		logger = newDevelopmentLogger()
	case EnvProd, EnvProduction:
		logger = newProductionLogger()
	default:
		logger = newDevelopmentLogger()
	}

	return &Logger{
		Logger:      logger,
		environment: env,
	}
}

func NewLogger(config Config) (*Logger, error) {
	var level zapcore.Level
	switch config.Level {
	case LevelDebug:
		level = zap.DebugLevel
	case LevelInfo:
		level = zap.InfoLevel
	case LevelWarn:
		level = zap.WarnLevel
	case LevelError:
		level = zap.ErrorLevel
	case LevelFatal:
		level = zap.FatalLevel
	default:
		level = zap.InfoLevel
	}

	var encoderConfig zapcore.EncoderConfig
	var encoder zapcore.Encoder

	if config.Format == "json" {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "timestamp"
		encoderConfig.LevelKey = "level"
		encoderConfig.MessageKey = "message"
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	var writeSyncer zapcore.WriteSyncer
	switch config.Output {
	case "stderr":
		writeSyncer = zapcore.AddSync(os.Stderr)
	case "file":
		if config.FilePath == "" {
			return nil, fmt.Errorf("file path is required when output is set to file")
		}
		file, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		writeSyncer = zapcore.AddSync(file)
	default:
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	core := zapcore.NewCore(encoder, writeSyncer, level)
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))

	return &Logger{
		Logger:      zapLogger,
		serviceName: config.ServiceName,
		environment: config.Environment,
	}, nil
}

func newDevelopmentLogger() *zap.Logger {
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	writeSyncer := zapcore.AddSync(os.Stdout)
	core := zapcore.NewCore(consoleEncoder, writeSyncer, zap.DebugLevel)

	return zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zap.ErrorLevel),
	)
}

func newProductionLogger() *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	encoderConfig.EncodeCaller = func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(caller.TrimmedPath())
	}

	config := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       false,
		Encoding:          "json",
		EncoderConfig:     encoderConfig,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
		DisableCaller:     false,
		DisableStacktrace: false,
	}

	logger, _ := config.Build()
	return logger
}

func (l *Logger) With(fields ...zapcore.Field) *Logger {
	return &Logger{
		Logger:      l.Logger.With(fields...),
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger:      l.Logger.With(zap.String("component", component)),
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

func (l *Logger) WithService(service string) *Logger {
	return &Logger{
		Logger:      l.Logger.With(zap.String("service", service)),
		serviceName: service,
		environment: l.environment,
	}
}

func (l *Logger) Sync() {
	_ = l.Logger.Sync()
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := make([]zapcore.Field, 0, 5)

	if l.serviceName != "" {
		fields = append(fields, zap.String("service", l.serviceName))
	}

	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		if rid, ok := requestID.(string); ok {
			fields = append(fields, zap.String("request_id", rid))
		}
	}
	if userID := ctx.Value(UserIDKey); userID != nil {
		if uid, ok := userID.(string); ok {
			fields = append(fields, zap.String("user_id", uid))
		}
	}
	if operation := ctx.Value(OperationKey); operation != nil {
		if op, ok := operation.(string); ok {
			fields = append(fields, zap.String("operation", op))
		}
	}
	if traceID := ctx.Value(TraceIDKey); traceID != nil {
		if tid, ok := traceID.(string); ok {
			fields = append(fields, zap.String("trace_id", tid))
		}
	}

	return &Logger{
		Logger:      l.Logger.With(fields...),
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

func (l *Logger) WithFields(fields Fields) *Logger {
	zapFields := make([]zapcore.Field, 0, len(fields)+1)

	if l.serviceName != "" {
		zapFields = append(zapFields, zap.String("service", l.serviceName))
	}

	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	return &Logger{
		Logger:      l.Logger.With(zapFields...),
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

func (l *Logger) WithError(err error) *Logger {
	fields := []zapcore.Field{zap.Error(err)}
	if l.serviceName != "" {
		fields = append(fields, zap.String("service", l.serviceName))
	}

	return &Logger{
		Logger:      l.Logger.With(fields...),
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

func (l *Logger) LogRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	l.WithContext(ctx).Info("HTTP request processed",
		zap.String("http_method", method),
		zap.String("http_path", path),
		zap.Int("http_status", statusCode),
		zap.Int64("response_time_ms", duration.Milliseconds()),
		zap.String("type", "http_request"),
	)
}

func (l *Logger) LogGRPCRequest(ctx context.Context, method string, duration time.Duration, err error) {
	logger := l.WithContext(ctx)
	fields := []zapcore.Field{
		zap.String("grpc_method", method),
		zap.Int64("response_time_ms", duration.Milliseconds()),
		zap.String("type", "grpc_request"),
	}

	if err != nil {
		logger.Error("gRPC request failed", append(fields, zap.Error(err))...)
	} else {
		logger.Info("gRPC request processed", fields...)
	}
}

func (l *Logger) LogDatabaseQuery(ctx context.Context, query string, duration time.Duration, err error) {
	logger := l.WithContext(ctx)
	fields := []zapcore.Field{
		zap.String("db_query", query),
		zap.Int64("query_time_ms", duration.Milliseconds()),
		zap.String("type", "database_query"),
	}

	if err != nil {
		logger.Error("Database query failed", append(fields, zap.Error(err))...)
	} else {
		logger.Debug("Database query executed", fields...)
	}
}

func (l *Logger) LogCacheOperation(ctx context.Context, operation, key string, hit bool, duration time.Duration) {
	l.WithContext(ctx).Debug("Cache operation",
		zap.String("cache_operation", operation),
		zap.String("cache_key", key),
		zap.Bool("cache_hit", hit),
		zap.Int64("operation_time_ms", duration.Milliseconds()),
		zap.String("type", "cache_operation"),
	)
}

func (l *Logger) LogBusinessEvent(ctx context.Context, event string, data interface{}) {
	dataJSON, _ := json.Marshal(data)
	l.WithContext(ctx).Info("Business event occurred",
		zap.String("business_event", event),
		zap.String("event_data", string(dataJSON)),
		zap.String("type", "business_event"),
	)
}

func (l *Logger) LogSecurity(ctx context.Context, event, details string, severity string) {
	l.WithContext(ctx).Warn("Security event",
		zap.String("security_event", event),
		zap.String("security_details", details),
		zap.String("severity", severity),
		zap.String("type", "security_event"),
	)
}

func Err(err error) zapcore.Field {
	return zap.Error(err)
}

func Discard() *Logger {
	return &Logger{zap.NewNop(), "", ""}
}

func Test() *Logger {
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(os.Stderr),
		zap.NewAtomicLevelAt(zap.DebugLevel),
	)
	return &Logger{zap.New(core), "test", "test"}
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
