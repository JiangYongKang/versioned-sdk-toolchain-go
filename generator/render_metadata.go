package generator

import (
	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
)

// metadataOperation is the machine-readable call metadata for one op.
// Types are emitted as stable, self-describing JSON objects identical in
// shape to the loader DTO so other tools can consume them without Go.
type metadataType struct {
	Kind   string          `json:"kind"`
	Ref    string          `json:"ref,omitempty"`
	Elem   *metadataType   `json:"elem,omitempty"`
	Values []string        `json:"values,omitempty"`
	Fields []metadataField `json:"fields,omitempty"`
}

type metadataField struct {
	Name     string       `json:"name"`
	Optional bool         `json:"optional,omitempty"`
	Doc      string       `json:"doc,omitempty"`
	Type     metadataType `json:"type"`
}

type metadataOperation struct {
	Name   string       `json:"name"`
	Doc    string       `json:"doc,omitempty"`
	Input  metadataType `json:"input"`
	Output metadataType `json:"output"`
}

type metadataNamedType struct {
	Name string       `json:"name"`
	Doc  string       `json:"doc,omitempty"`
	Type metadataType `json:"type"`
}

type metadataFile struct {
	Name       string              `json:"name"`
	Version    string              `json:"version"`
	Codec      string              `json:"codec"`
	Doc        string              `json:"doc,omitempty"`
	Operations []metadataOperation `json:"operations"`
	Types      []metadataNamedType `json:"types"`
}

func metaType(t schema.Type) metadataType {
	m := metadataType{Kind: string(t.Kind)}
	switch t.Kind {
	case schema.TypeRef:
		m.Ref = t.RefName
	case schema.TypeEnum:
		m.Values = append([]string(nil), t.EnumVals...)
	case schema.TypeArray, schema.TypeMap:
		if t.Elem != nil {
			e := metaType(*t.Elem)
			m.Elem = &e
		}
	case schema.TypeObject:
		for _, f := range t.Fields {
			m.Fields = append(m.Fields, metadataField{
				Name: f.Name, Optional: f.Optional, Doc: f.Doc, Type: metaType(f.Type),
			})
		}
	}
	return m
}

func buildMetadata(iface *schema.Interface) *metadataFile {
	mf := &metadataFile{
		Name: iface.Name, Version: iface.Version, Codec: iface.Codec, Doc: iface.Doc,
	}
	for _, nt := range iface.Types {
		mf.Types = append(mf.Types, metadataNamedType{
			Name: nt.Name, Doc: nt.Doc, Type: metaType(nt.Type),
		})
	}
	for _, op := range iface.Operations {
		mf.Operations = append(mf.Operations, metadataOperation{
			Name: op.Name, Doc: op.Doc,
			Input:  metaType(op.Input),
			Output: metaType(op.Output),
		})
	}
	return mf
}
