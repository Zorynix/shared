package errors

import (
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ErrorCode string

const (
	ErrUnauthorized       ErrorCode = "UNAUTHORIZED"
	ErrInvalidToken       ErrorCode = "INVALID_TOKEN"
	ErrTokenExpired       ErrorCode = "TOKEN_EXPIRED"
	ErrInvalidCredentials ErrorCode = "INVALID_CREDENTIALS"

	ErrValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrMissingRequired  ErrorCode = "MISSING_REQUIRED_FIELD"

	ErrDatabaseConnection ErrorCode = "DATABASE_CONNECTION"
	ErrRecordNotFound     ErrorCode = "RECORD_NOT_FOUND"
	ErrDuplicateRecord    ErrorCode = "DUPLICATE_RECORD"
	ErrDatabaseQuery      ErrorCode = "DATABASE_QUERY_ERROR"

	ErrCacheConnection   ErrorCode = "CACHE_CONNECTION"
	ErrCacheKeyNotFound  ErrorCode = "CACHE_KEY_NOT_FOUND"
	ErrCacheInvalidation ErrorCode = "CACHE_INVALIDATION_ERROR"

	ErrServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrRateLimitExceeded  ErrorCode = "RATE_LIMIT_EXCEEDED"
	ErrInternalServer     ErrorCode = "INTERNAL_SERVER_ERROR"
	ErrCircuitBreakerOpen ErrorCode = "CIRCUIT_BREAKER_OPEN"

	ErrUserNotFound            ErrorCode = "USER_NOT_FOUND"
	ErrTestNotFound            ErrorCode = "TEST_NOT_FOUND"
	ErrInsufficientPermissions ErrorCode = "INSUFFICIENT_PERMISSIONS"
	ErrResourceLocked          ErrorCode = "RESOURCE_LOCKED"
)

type AppError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	Details    string                 `json:"details,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Cause      error                  `json:"-"`
	HTTPStatus int                    `json:"-"`
	GRPCStatus codes.Code             `json:"-"`
}

func (e *AppError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func (e *AppError) WithMetadata(key string, value interface{}) *AppError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

func (e *AppError) ToGRPCStatus() *status.Status {
	return status.New(e.GRPCStatus, e.Message)
}

func NewAppError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: getHTTPStatusForCode(code),
		GRPCStatus: getGRPCStatusForCode(code),
	}
}

func NewAppErrorWithDetails(code ErrorCode, message, details string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		Details:    details,
		HTTPStatus: getHTTPStatusForCode(code),
		GRPCStatus: getGRPCStatusForCode(code),
	}
}

var (
	ErrUnauthorizedAccess     = NewAppError(ErrUnauthorized, "Unauthorized access")
	ErrInvalidTokenFormat     = NewAppError(ErrInvalidToken, "Invalid token format")
	ErrTokenHasExpired        = NewAppError(ErrTokenExpired, "Token has expired")
	ErrInvalidUserCredentials = NewAppError(ErrInvalidCredentials, "Invalid user credentials")

	ErrUserAlreadyExists = NewAppError(ErrDuplicateRecord, "User already exists")
	ErrUserNotFoundError = NewAppError(ErrUserNotFound, "User not found")
	ErrTestNotFoundError = NewAppError(ErrTestNotFound, "Test not found")

	ErrDatabaseConnectionFailed = NewAppError(ErrDatabaseConnection, "Database connection failed")
	ErrCacheConnectionFailed    = NewAppError(ErrCacheConnection, "Cache connection failed")

	ErrServiceTemporarilyUnavailable = NewAppError(ErrServiceUnavailable, "Service temporarily unavailable")
	ErrRateLimitExceededError        = NewAppError(ErrRateLimitExceeded, "Rate limit exceeded")
)

func getHTTPStatusForCode(code ErrorCode) int {
	switch code {
	case ErrUnauthorized, ErrInvalidToken, ErrTokenExpired, ErrInvalidCredentials:
		return http.StatusUnauthorized
	case ErrValidationFailed, ErrInvalidInput, ErrMissingRequired:
		return http.StatusBadRequest
	case ErrRecordNotFound, ErrUserNotFound, ErrTestNotFound:
		return http.StatusNotFound
	case ErrDuplicateRecord:
		return http.StatusConflict
	case ErrInsufficientPermissions:
		return http.StatusForbidden
	case ErrRateLimitExceeded:
		return http.StatusTooManyRequests
	case ErrServiceUnavailable, ErrCircuitBreakerOpen:
		return http.StatusServiceUnavailable
	case ErrResourceLocked:
		return http.StatusLocked
	default:
		return http.StatusInternalServerError
	}
}

func getGRPCStatusForCode(code ErrorCode) codes.Code {
	switch code {
	case ErrUnauthorized, ErrInvalidToken, ErrTokenExpired, ErrInvalidCredentials:
		return codes.Unauthenticated
	case ErrValidationFailed, ErrInvalidInput, ErrMissingRequired:
		return codes.InvalidArgument
	case ErrRecordNotFound, ErrUserNotFound, ErrTestNotFound:
		return codes.NotFound
	case ErrDuplicateRecord:
		return codes.AlreadyExists
	case ErrInsufficientPermissions:
		return codes.PermissionDenied
	case ErrRateLimitExceeded:
		return codes.ResourceExhausted
	case ErrServiceUnavailable, ErrCircuitBreakerOpen:
		return codes.Unavailable
	case ErrDatabaseConnection, ErrCacheConnection:
		return codes.Internal
	default:
		return codes.Internal
	}
}
