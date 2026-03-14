package mobile

import "fmt"

// Error codes for structured error reporting to native callers.
const (
	ErrAlreadyRunning = 1
	ErrNotRunning     = 2
	ErrInvalidConfig  = 3
	ErrStartFailed    = 4
	ErrReloadFailed   = 5
)

// MobileError is a structured error with a numeric code for native consumers.
// gomobile exports this as a class with Code and Message fields.
type MobileError struct {
	Code    int
	Message string
}

func (e *MobileError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// NewMobileError creates a new MobileError with the given code and message.
func NewMobileError(code int, message string) *MobileError {
	return &MobileError{Code: code, Message: message}
}
