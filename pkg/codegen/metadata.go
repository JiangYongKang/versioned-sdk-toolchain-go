package codegen

import (
	"encoding/json"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

// Metadata is the machine-readable call metadata emitted next to the SDK.
// Field order and JSON key order are fixed so repeated generations produce
// byte-identical output.
type Metadata struct {
	Tool       string          `json:"tool"`
	Service    string          `json:"service"`
	Version    string          `json:"version"`
	Format     string          `json:"format"`
	Interfaces []MetaInterface `json:"interfaces"`
	Enums      []MetaEnum      `json:"enums"`
	Structs    []MetaStruct    `json:"structs"`
	Artifacts  []string        `json:"artifacts"`
}

// MetaType is the serializable form of a type reference.
type MetaType struct {
	Kind      string    `json:"kind"`
	Name      string    `json:"name,omitempty"`
	Primitive string    `json:"primitive,omitempty"`
	Element   *MetaType `json:"element,omitempty"`
	Value     *MetaType `json:"value,omitempty"`
	Nullable  bool      `json:"nullable,omitempty"`
}

// MetaField describes a struct field in metadata.
type MetaField struct {
	Name     string   `json:"name"`
	Type     MetaType `json:"type"`
	Optional bool     `json:"optional,omitempty"`
}

// MetaParam describes an interface parameter in metadata.
type MetaParam struct {
	Name     string   `json:"name"`
	Type     MetaType `json:"type"`
	Optional bool     `json:"optional,omitempty"`
}

// MetaInterface describes one callable operation in metadata.
type MetaInterface struct {
	Name   string      `json:"name"`
	Method string      `json:"method"`
	Input  []MetaParam `json:"input"`
	Output *MetaType   `json:"output,omitempty"`
	Errors []MetaType  `json:"errors,omitempty"`
}

// MetaEnum describes an enum in metadata.
type MetaEnum struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// MetaStruct describes a struct in metadata.
type MetaStruct struct {
	Name   string      `json:"name"`
	Fields []MetaField `json:"fields"`
}

func toMetaType(t schema.TypeRef) MetaType {
	mt := MetaType{Kind: t.Kind(), Nullable: t.Nullable}
	switch t.Kind() {
	case "primitive":
		mt.Primitive = string(t.Primitive)
	case "ref":
		mt.Name = t.Name
	case "array":
		inner := toMetaType(*t.Array)
		mt.Element = &inner
	case "map":
		inner := toMetaType(*t.MapValue)
		mt.Value = &inner
	}
	return mt
}

// buildMetadata projects a bundle into deterministic metadata.
func buildMetadata(b *schema.Bundle, format string, artifacts []string) []byte {
	md := Metadata{
		Tool:      "versioned-sdk-toolchain-go",
		Service:   b.Documents[0].Service,
		Version:   b.Documents[0].Version.String(),
		Format:    format,
		Artifacts: append([]string(nil), artifacts...),
	}
	seen := map[string]bool{}
	for _, d := range b.Documents {
		for _, ifc := range d.Interfaces {
			mi := MetaInterface{Name: ifc.Name, Method: ifc.Method}
			for _, p := range ifc.Input {
				mi.Input = append(mi.Input, MetaParam{
					Name: p.Name, Type: toMetaType(p.Type), Optional: p.Optional,
				})
			}
			if ifc.Output != nil {
				o := toMetaType(*ifc.Output)
				mi.Output = &o
			}
			for _, e := range ifc.Errors {
				mi.Errors = append(mi.Errors, toMetaType(e))
			}
			md.Interfaces = append(md.Interfaces, mi)
		}
		for _, e := range d.Enums {
			if seen[e.Name] {
				continue
			}
			seen[e.Name] = true
			md.Enums = append(md.Enums, MetaEnum{Name: e.Name, Values: append([]string(nil), e.Values...)})
		}
		for _, s := range d.Structs {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			ms := MetaStruct{Name: s.Name}
			for _, f := range s.Fields {
				ms.Fields = append(ms.Fields, MetaField{
					Name: f.Name, Type: toMetaType(f.Type), Optional: f.Optional,
				})
			}
			md.Structs = append(md.Structs, ms)
		}
	}
	// Stable ordering for duplicate-free merged collections.
	sortMeta(&md)
	data, _ := json.MarshalIndent(md, "", "  ")
	return append(data, '\n')
}
