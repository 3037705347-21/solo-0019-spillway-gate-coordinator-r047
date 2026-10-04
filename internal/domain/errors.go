package domain

import "errors"

type ErrorKind string

const (
	KindInvalid  ErrorKind = "invalid"
	KindNotFound ErrorKind = "not_found"
	KindConflict ErrorKind = "conflict"
	KindInternal ErrorKind = "internal"
)

type Error struct {
	Kind    ErrorKind
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

func Invalid(code, message string) error {
	return &Error{Kind: KindInvalid, Code: code, Message: message}
}

func NotFound(code, message string) error {
	return &Error{Kind: KindNotFound, Code: code, Message: message}
}

func Conflict(code, message string) error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

func Internal(code, message string) error {
	return &Error{Kind: KindInternal, Code: code, Message: message}
}

func ErrorKindOf(err error) ErrorKind {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind
	}
	return KindInternal
}

func ErrorCodeOf(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return "internal_error"
}

func ErrorMessageOf(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Message
	}
	return "internal server error"
}
