package load

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

// Loader reads local interface description documents (JSON files).
type Loader struct {
	root fs.FS
}

// New creates a Loader that reads from the host filesystem.
func New() *Loader {
	return &Loader{root: nil}
}

// NewFS creates a Loader rooted at f (useful for in-memory test inputs).
func NewFS(f fs.FS) *Loader {
	return &Loader{root: f}
}

func (l *Loader) readFile(path string) ([]byte, error) {
	if l.root != nil {
		return fs.ReadFile(l.root, path)
	}
	return os.ReadFile(path)
}

// wireRef is the on-disk shape of a type reference. Exactly one key must
// identify the shape: "ref", "primitive", "array" or "map".
type wireRef struct {
	Ref       string   `json:"ref"`
	Primitive string   `json:"primitive"`
	Array     *wireRef `json:"array"`
	Map       *wireRef `json:"map"`
	Nullable  bool     `json:"nullable"`
}

func decodeRef(w *wireRef) (schema.TypeRef, error) {
	set := 0
	if w.Ref != "" {
		set++
	}
	if w.Primitive != "" {
		set++
	}
	if w.Array != nil {
		set++
	}
	if w.Map != nil {
		set++
	}
	if set != 1 {
		return schema.TypeRef{}, fmt.Errorf("type reference must set exactly one of ref/primitive/array/map, got %d", set)
	}
	t := schema.TypeRef{Nullable: w.Nullable}
	switch {
	case w.Ref != "":
		t.Name = w.Ref
	case w.Primitive != "":
		t.Primitive = schema.Primitive(w.Primitive)
	case w.Array != nil:
		inner, err := decodeRef(w.Array)
		if err != nil {
			return schema.TypeRef{}, fmt.Errorf("array element: %w", err)
		}
		t.Array = &inner
	case w.Map != nil:
		inner, err := decodeRef(w.Map)
		if err != nil {
			return schema.TypeRef{}, fmt.Errorf("map value: %w", err)
		}
		t.MapValue = &inner
	}
	return t, nil
}

type wireField struct {
	Name     string   `json:"name"`
	Type     *wireRef `json:"type"`
	Optional bool     `json:"optional"`
}

type wireParam struct {
	Name     string   `json:"name"`
	Type     *wireRef `json:"type"`
	Optional bool     `json:"optional"`
}

type wireIface struct {
	Name   string      `json:"name"`
	Method string      `json:"method"`
	Input  []wireParam `json:"input"`
	Output *wireRef    `json:"output"`
	Errors []*wireRef  `json:"errors"`
}

type wireEnum struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type wireStruct struct {
	Name   string      `json:"name"`
	Fields []wireField `json:"fields"`
}

type wireDoc struct {
	Service    string       `json:"service"`
	Version    string       `json:"version"`
	Interfaces []wireIface  `json:"interfaces"`
	Enums      []wireEnum   `json:"enums"`
	Structs    []wireStruct `json:"structs"`
}

func (l *Loader) loadOne(path string) (*schema.Document, error) {
	data, err := l.readFile(path)
	if err != nil {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			fmt.Sprintf("cannot read description file: %s", err)).
			WithOp("load").WithField(path)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w wireDoc
	if err := dec.Decode(&w); err != nil {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			fmt.Sprintf("invalid JSON in %s: %s", filepath.Base(path), err)).
			WithOp("load").WithField(path)
	}
	if dec.More() {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			"file contains multiple JSON values; expected one document").
			WithOp("load").WithField(path)
	}
	if w.Service == "" {
		return nil, apierrors.New(apierrors.CatInvalidInput, "missing \"service\"").
			WithOp("load").WithField(path)
	}
	ver, err := schema.ParseVersion(w.Version)
	if err != nil {
		return nil, apierrors.New(apierrors.CatInvalidInput, err.Error()).
			WithOp("load").WithField(path)
	}
	doc := &schema.Document{Service: w.Service, Version: ver, Source: path}

	for _, e := range w.Enums {
		en := schema.Enum{Name: e.Name, Values: append([]string(nil), e.Values...)}
		doc.Enums = append(doc.Enums, en)
	}
	for _, s := range w.Structs {
		st := schema.Struct{Name: s.Name}
		for _, f := range s.Fields {
			if f.Type == nil {
				return nil, apierrors.New(apierrors.CatInvalidInput,
					fmt.Sprintf("struct %q field %q has no type", s.Name, f.Name)).
					WithOp("load").WithField(path)
			}
			tr, err := decodeRef(f.Type)
			if err != nil {
				return nil, apierrors.New(apierrors.CatInvalidInput,
					fmt.Sprintf("struct %q field %q: %s", s.Name, f.Name, err)).
					WithOp("load").WithField(path)
			}
			st.Fields = append(st.Fields, schema.Field{Name: f.Name, Type: tr, Optional: f.Optional})
		}
		doc.Structs = append(doc.Structs, st)
	}
	for _, i := range w.Interfaces {
		ifc := schema.Interface{Name: i.Name, Method: i.Method}
		for _, p := range i.Input {
			if p.Type == nil {
				return nil, apierrors.New(apierrors.CatInvalidInput,
					fmt.Sprintf("interface %q parameter %q has no type", i.Name, p.Name)).
					WithOp("load").WithField(path).WithInterface(i.Name)
			}
			tr, err := decodeRef(p.Type)
			if err != nil {
				return nil, apierrors.New(apierrors.CatInvalidInput,
					fmt.Sprintf("interface %q parameter %q: %s", i.Name, p.Name, err)).
					WithOp("load").WithField(path).WithInterface(i.Name)
			}
			ifc.Input = append(ifc.Input, schema.Parameter{Name: p.Name, Type: tr, Optional: p.Optional})
		}
		if i.Output != nil {
			tr, err := decodeRef(i.Output)
			if err != nil {
				return nil, apierrors.New(apierrors.CatInvalidInput,
					fmt.Sprintf("interface %q output: %s", i.Name, err)).
					WithOp("load").WithField(path).WithInterface(i.Name)
			}
			ifc.Output = &tr
		}
		for j, er := range i.Errors {
			if er == nil {
				return nil, apierrors.New(apierrors.CatInvalidInput,
					fmt.Sprintf("interface %q error #%d has no type", i.Name, j)).
					WithOp("load").WithField(path).WithInterface(i.Name)
			}
			tr, err := decodeRef(er)
			if err != nil {
				return nil, apierrors.New(apierrors.CatInvalidInput,
					fmt.Sprintf("interface %q error #%d: %s", i.Name, j, err)).
					WithOp("load").WithField(path).WithInterface(i.Name)
			}
			ifc.Errors = append(ifc.Errors, tr)
		}
		doc.Interfaces = append(doc.Interfaces, ifc)
	}
	return doc, nil
}

// LoadFiles reads and decodes the given local files into a Bundle. The
// documents are ordered deterministically by source path so repeated runs
// over the same input are stable.
func (l *Loader) LoadFiles(paths ...string) (*schema.Bundle, error) {
	docs := make([]*schema.Document, 0, len(paths))
	for _, p := range paths {
		doc, err := l.loadOne(p)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].Source < docs[j].Source })
	return &schema.Bundle{Documents: docs}, nil
}
