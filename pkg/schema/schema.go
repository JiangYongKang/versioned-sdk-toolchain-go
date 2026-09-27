package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a semantic version (major.minor.patch).
type Version struct {
	Major int
	Minor int
	Patch int
	raw   string
}

// ParseVersion parses a semantic version string of the form major.minor.patch
// where each component is a non-negative integer without leading zeros
// (except the single digit 0).
func ParseVersion(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected major.minor.patch", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return Version{}, fmt.Errorf("invalid version %q: malformed component %q", s, p)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("invalid version %q: non-numeric component %q", s, p)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], raw: s}, nil
}

// String renders the version.
func (v Version) String() string {
	if v.raw != "" {
		return v.raw
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1, 0 or 1 depending on whether v is less than, equal to
// or greater than other.
func (v Version) Compare(other Version) int {
	switch {
	case v.Major != other.Major:
		if v.Major < other.Major {
			return -1
		}
		return 1
	case v.Minor != other.Minor:
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	case v.Patch != other.Patch:
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	default:
		return 0
	}
}

// Primitive identifies a built-in scalar type.
type Primitive string

// Supported primitive type names.
const (
	PrimString    Primitive = "string"
	PrimInt       Primitive = "int"
	PrimLong      Primitive = "long"
	PrimFloat     Primitive = "float"
	PrimDouble    Primitive = "double"
	PrimBool      Primitive = "bool"
	PrimBytes     Primitive = "bytes"
	PrimTimestamp Primitive = "timestamp"
)

// IsPrimitive reports whether p names a supported scalar type.
func IsPrimitive(p Primitive) bool {
	switch p {
	case PrimString, PrimInt, PrimLong, PrimFloat, PrimDouble,
		PrimBool, PrimBytes, PrimTimestamp:
		return true
	}
	return false
}

// TypeRef references a named user type, a primitive, an array or a map.
// Exactly one of Primitive/Name/Array/MapValue is set.
type TypeRef struct {
	Name      string
	Primitive Primitive
	Array     *TypeRef
	MapValue  *TypeRef
	Nullable  bool
}

// String renders the reference in a stable, readable form.
func (t TypeRef) String() string {
	base := ""
	switch {
	case t.Primitive != "":
		base = string(t.Primitive)
	case t.Name != "":
		base = t.Name
	case t.Array != nil:
		base = "[]" + t.Array.String()
	case t.MapValue != nil:
		base = "map[string]" + t.MapValue.String()
	default:
		base = "<unknown>"
	}
	if t.Nullable {
		return base + "?"
	}
	return base
}

// Kind reports which shape the reference has: "primitive", "ref", "array",
// "map" or "unknown".
func (t TypeRef) Kind() string {
	switch {
	case t.Primitive != "":
		return "primitive"
	case t.Name != "":
		return "ref"
	case t.Array != nil:
		return "array"
	case t.MapValue != nil:
		return "map"
	default:
		return "unknown"
	}
}

// Field is a struct field.
type Field struct {
	Name     string
	Type     TypeRef
	Optional bool
}

// Enum is a closed set of string values.
type Enum struct {
	Name   string
	Values []string
}

// Struct is a named record type.
type Struct struct {
	Name   string
	Fields []Field
}

// Parameter is an interface input parameter.
type Parameter struct {
	Name     string
	Type     TypeRef
	Optional bool
}

// Interface describes one callable operation.
type Interface struct {
	Name   string
	Method string
	Input  []Parameter
	Output *TypeRef
	Errors []TypeRef
}

// Document is one local interface description file.
type Document struct {
	Service    string
	Version    Version
	Source     string
	Interfaces []Interface
	Enums      []Enum
	Structs    []Struct
}

// Bundle is the resolved set of documents used as generator input.
type Bundle struct {
	Documents []*Document
}

// FindStruct looks up a struct by name across documents. It returns nil and
// an empty service when the name is not defined anywhere.
func (b *Bundle) FindStruct(name string) (*Struct, string) {
	for _, d := range b.Documents {
		for i := range d.Structs {
			if d.Structs[i].Name == name {
				return &d.Structs[i], d.Service
			}
		}
	}
	return nil, ""
}

// FindEnum looks up an enum by name across documents.
func (b *Bundle) FindEnum(name string) (*Enum, string) {
	for _, d := range b.Documents {
		for i := range d.Enums {
			if d.Enums[i].Name == name {
				return &d.Enums[i], d.Service
			}
		}
	}
	return nil, ""
}

// FindInterface looks up an operation by name across documents.
func (b *Bundle) FindInterface(name string) (*Interface, string) {
	for _, d := range b.Documents {
		for i := range d.Interfaces {
			if d.Interfaces[i].Name == name {
				return &d.Interfaces[i], d.Service
			}
		}
	}
	return nil, ""
}
