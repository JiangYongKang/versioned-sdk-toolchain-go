// Package sdkerrors defines the unified error taxonomy used across the
// schema-driven SDK toolchain and the generated runtime SDK.
//
// All errors surfaced to callers must be an *Error carrying a stable Kind
// so that callers can distinguish:
//
//   - KindInput       : malformed interface description / bad arguments
//   - KindVersion     : incompatible cross-version change or bad version
//   - KindGeneration  : SDK artifact generation failure
//   - KindRuntime      : failure while invoking a generated SDK operation
//
// Errors wrap underlying failures with Unwrap but never let foreign error
// strings leak through the stable Kind classification: the stable fields
// (Kind/Reason/Service/Version/Field/Op) are always authored by this
// toolchain; the foreign cause is only available via Unwrap/Is.
package sdkerrors

import (
	"errors"
	"fmt"
	"strings"
)

// Kind is the stable error category.
type Kind string

// Stable error kinds.
const (
	KindInput      Kind = "input_error"
	KindVersion    Kind = "version_incompatible"
	KindGeneration Kind = "generation_failed"
	KindRuntime    Kind = "runtime_call_failed"
)

// Error is the structured error type used everywhere in the toolchain.
type Error struct {
	Kind    Kind
	Reason  string
	Service string
	Version string
	Field   string
	Op      string
	Cause   error
}

// Error renders a stable, deterministic, human-readable message. The
// ordering of fields is fixed so assertions and golden output are stable.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Kind))
	b.WriteString(": ")
	b.WriteString(e.Reason)

	var loc []string
	if e.Service != "" {
		loc = append(loc, "service="+e.Service)
	}
	if e.Version != "" {
		loc = append(loc, "version="+e.Version)
	}
	if e.Op != "" {
		loc = append(loc, "op="+e.Op)
	}
	if e.Field != "" {
		loc = append(loc, "field="+e.Field)
	}
	if len(loc) > 0 {
		b.WriteString(" [")
		b.WriteString(strings.Join(loc, ", "))
		b.WriteString("]")
	}
	if e.Cause != nil {
		b.WriteString(": ")
		b.WriteString(e.Cause.Error())
	}
	return b.String()
}

// Unwrap returns the wrapped cause.
func (e *Error) Unwrap() error { return e.Cause }

// New creates an *Error with the given kind and reason.
func New(kind Kind, reason string) *Error {
	return &Error{Kind: kind, Reason: reason}
}

// Newf creates an *Error with a formatted reason.
func Newf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Reason: fmt.Sprintf(format, args...)}
}

// WithService annotates the error with a service name.
func (e *Error) WithService(name string) *Error {
	e.Service = name
	return e
}

// WithVersion annotates the error with a version.
func (e *Error) WithVersion(v string) *Error {
	e.Version = v
	return e
}

// WithField annotates the error with a fully qualified field path.
func (e *Error) WithField(path string) *Error {
	e.Field = path
	return e
}

// WithOp annotates the error with an operation name.
func (e *Error) WithOp(op string) *Error {
	e.Op = op
	return e
}

// Wrap attaches an underlying cause.
func (e *Error) Wrap(cause error) *Error {
	e.Cause = cause
	return e
}

// IsKind reports whether err (or any wrapped *Error) has the given kind.
func IsKind(err error, kind Kind) bool {
	var se *Error
	if errors.As(err, &se) {
		return se.Kind == kind
	}
	return false
}

// AsError extracts the first *Error in err's chain, or nil.
func AsError(err error) *Error {
	var se *Error
	if errors.As(err, &se) {
		return se
	}
	return nil
}
