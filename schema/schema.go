// Package schema defines the in-memory model of interface descriptions
// consumed by the toolchain.
package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// TypeKind enumerates the kinds of type definitions.
type TypeKind string

// Supported type definition kinds.
const (
	KindString  TypeKind = "string"
	KindInteger TypeKind = "integer"
	KindNumber  TypeKind = "number"
	KindBoolean TypeKind = "boolean"
	TypeObject  TypeKind = "object"
	TypeEnum    TypeKind = "enum"
	TypeArray   TypeKind = "array"
	TypeMap     TypeKind = "map"
	TypeRef     TypeKind = "ref"
)

// Type is a type reference used by fields/parameters.
//
// Primitive kinds (string/integer/number/boolean) carry no extra data.
// Array and Map carry Elem; Ref carries RefName; Enum carries EnumVals;
// Object carries Fields.
type Type struct {
	Kind     TypeKind
	RefName  string
	Elem     *Type
	EnumVals []string
	Fields   []Field
}

// Field is one member of an object.
type Field struct {
	Name     string `json:"name"`
	Type     Type   `json:"type"`
	Optional bool   `json:"optional,omitempty"`
	Doc      string `json:"doc,omitempty"`
}

// NamedType is a named, reusable type declared by an interface.
type NamedType struct {
	Name string
	Doc  string
	Type Type
}

// Operation describes a single RPC-style call.
type Operation struct {
	Name   string
	Doc    string
	Input  Type
	Output Type
}

// Interface is one service/interface description.
type Interface struct {
	Name       string
	Version    string
	Codec      string
	Doc        string
	Operations []Operation
	Types      []NamedType
}

// Bundle is one (service, version) parsed description plus its source path.
type Bundle struct {
	Interface *Interface
	Source    string
}

// Document is a collection of bundles loaded together (one batch).
type Document struct {
	Bundles []*Bundle
}

// Version is a parsed semantic version with an optional leading "v".
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseVersion parses forms like "v1", "1", "v1.2", "v1.2.3". It rejects
// empty values, negative numbers, non-numeric components and extra
// segments so illegal versions never enter the pipeline silently.
func ParseVersion(s string) (Version, error) {
	v := Version{}
	raw := strings.TrimSpace(s)
	if raw == "" {
		return v, fmt.Errorf("empty version")
	}
	if raw[0] == 'v' || raw[0] == 'V' {
		raw = raw[1:]
	}
	if raw == "" {
		return v, fmt.Errorf("version %q has no numeric component", s)
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 3 {
		return v, fmt.Errorf("version %q has too many components", s)
	}
	nums := []*int{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		if p == "" {
			return v, fmt.Errorf("version %q has an empty component", s)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("version %q has non-numeric or negative component %q", s, p)
		}
		*nums[i] = n
	}
	return v, nil
}

// String renders the canonical, stable form vMAJOR.MINOR.PATCH.
func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1, 0 or 1 like strings.Compare.
func (v Version) Compare(o Version) int {
	switch {
	case v.Major != o.Major:
		return boolCmp(v.Major < o.Major)
	case v.Minor != o.Minor:
		return boolCmp(v.Minor < o.Minor)
	case v.Patch != o.Patch:
		return boolCmp(v.Patch < o.Patch)
	default:
		return 0
	}
}

func boolCmp(less bool) int {
	if less {
		return -1
	}
	return 1
}
