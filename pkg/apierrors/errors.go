package apierrors

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Category is the stable error class callers can switch on.
type Category string

// Stable error categories.
const (
	CatInvalidInput        Category = "invalid_input"
	CatVersionIncompatible Category = "version_incompatible"
	CatGenerationFailed    Category = "generation_failed"
	CatInvocationFailed    Category = "invocation_failed"
)

// Error carries a stable category plus precise interface/field/reason context.
type Error struct {
	Category  Category
	Op        string
	Interface string
	Field     string
	Reason    string
	Cause     error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Category))
	if e.Op != "" {
		fmt.Fprintf(&b, ": %s", e.Op)
	}
	if e.Interface != "" {
		fmt.Fprintf(&b, ": interface=%q", e.Interface)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, ": field=%q", e.Field)
	}
	if e.Reason != "" {
		fmt.Fprintf(&b, ": %s", e.Reason)
	}
	if e.Cause != nil {
		fmt.Fprintf(&b, ": caused by %s", e.Cause.Error())
	}
	return b.String()
}

// Unwrap exposes the wrapped cause to errors.Is/errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// New builds a categorized error.
func New(cat Category, reason string) *Error {
	return &Error{Category: cat, Reason: reason}
}

// WithInterface attaches an interface name.
func (e *Error) WithInterface(name string) *Error { e.Interface = name; return e }

// WithField attaches a field path.
func (e *Error) WithField(path string) *Error { e.Field = path; return e }

// WithOp attaches an operation name.
func (e *Error) WithOp(op string) *Error { e.Op = op; return e }

// WithCause attaches an underlying error without leaking its category.
func (e *Error) WithCause(err error) *Error { e.Cause = err; return e }

// Code returns the stable category of an error. Unknown errors map to
// CatInvocationFailed, nil maps to the empty string.
func Code(err error) Category {
	if err == nil {
		return ""
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Category
	}
	var me *MultiError
	if errors.As(err, &me) {
		return me.Category
	}
	return CatInvocationFailed
}

// Issue is one precise validation/compatibility finding.
type Issue struct {
	Interface string
	Field     string
	Reason    string
}

// MultiError aggregates multiple findings under one category.
type MultiError struct {
	Category Category
	Op       string
	Issues   []Issue
}

func (m *MultiError) Error() string {
	var b strings.Builder
	b.WriteString(string(m.Category))
	if m.Op != "" {
		fmt.Fprintf(&b, ": %s", m.Op)
	}
	fmt.Fprintf(&b, ": %d issue(s):\n", len(m.Issues))
	issues := append([]Issue(nil), m.Issues...)
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Interface != issues[j].Interface {
			return issues[i].Interface < issues[j].Interface
		}
		if issues[i].Field != issues[j].Field {
			return issues[i].Field < issues[j].Field
		}
		return issues[i].Reason < issues[j].Reason
	})
	for _, is := range issues {
		b.WriteString("  - ")
		if is.Interface != "" {
			fmt.Fprintf(&b, "interface=%q ", is.Interface)
		}
		if is.Field != "" {
			fmt.Fprintf(&b, "field=%q ", is.Field)
		}
		fmt.Fprintf(&b, "reason=%s\n", is.Reason)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Add appends a finding.
func (m *MultiError) Add(is Issue) { m.Issues = append(m.Issues, is) }

// Len reports the number of findings.
func (m *MultiError) Len() int { return len(m.Issues) }

// IsCategory reports whether err (or anything it wraps) carries the category.
func IsCategory(err error, cat Category) bool {
	return Code(err) == cat
}
