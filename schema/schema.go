// Package schema defines the in-memory model of interface descriptions
// consumed by the toolchain.
package schema

// TypeKind enumerates the kinds of type definitions.
type TypeKind string

// Supported type definition kinds.
const (
	KindString   TypeKind = "string"
	KindInteger  TypeKind = "integer"
	KindNumber   TypeKind = "number"
	KindBoolean  TypeKind = "boolean"
	TypeObject   TypeKind = "object"
	TypeEnum     TypeKind = "enum"
	TypeArray    TypeKind = "array"
	TypeMap      TypeKind = "map"
	TypeRef      TypeKind = "ref"
	TypeOptional TypeKind = "optional"
)

// Type is a type reference used by fields/parameters.
type Type struct {
	Kind     TypeKind
	RefName  string
	Elem     *Type
	EnumVals []string
	Fields   []Field
}

// Field is one member of an object.
type Field struct {
	Name     string
	Type     Type
	Optional bool
	Doc      string
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
	Doc        string
	Operations []Operation
	Types      []Type
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
