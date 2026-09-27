// Package compat decides whether interface changes across versions are
// backward compatible and refuses breaking changes with precise diagnostics.
package compat

import "github.com/highcumontoa/versioned-sdk-toolchain-go/schema"

// ChangeKind classifies a single detected difference.
type ChangeKind string

// Supported change classifications.
const (
	ChangeCompatible ChangeKind = "compatible"
	ChangeBreaking   ChangeKind = "breaking"
)

// Change describes one difference between two versions.
type Change struct {
	Kind    ChangeKind
	Service string
	FromVer string
	ToVer   string
	Op      string
	Field   string
	Detail  string
}

// Report is the result of comparing two versions.
type Report struct {
	Service string
	FromVer string
	ToVer   string
	Changes []Change
}

// Compatible reports whether no breaking changes were found.
func (r *Report) Compatible() bool { return false }

// Compare checks next against prev and records every change.
func Compare(prev, next *schema.Interface) (*Report, error) { return nil, nil }

// BreakingChanges returns only the breaking changes in a report.
func (r *Report) BreakingChanges() []Change { return nil }
