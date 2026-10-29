// Package fault defines public, deliberately non-sensitive error contracts.
package fault

import "errors"

type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Action      string `json:"action,omitempty"`
	cause       error
}

func (e *Error) Error() string        { return e.Code + ": " + e.Message }
func (e *Error) Unwrap() error        { return e.cause }
func New(code, message string) *Error { return &Error{Code: code, Message: message} }
func Wrap(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, cause: cause}
}
func Code(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "INTERNAL_ERROR"
}
func Public(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		v := *e
		v.cause = nil
		return &v
	}
	return New("INTERNAL_ERROR", "The operation failed; see the non-sensitive diagnostic report")
}
