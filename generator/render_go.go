package generator

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// tpl is the view model passed to the per-service Go template.
type tpl struct {
	Module     string
	Service    string
	Version    string
	VerNum     string // numeric version path component, e.g. "1_2_0"
	Codec      string
	Doc        string
	Types      []tplNamedType
	Operations []tplOperation
}

type tplNamedType struct {
	GoName string
	Doc    string
	Decl   string // "type X struct {...}" or "type X string"
}

type tplOperation struct {
	Name   string
	Doc    string
	Input  string // Go type name of input
	Output string // Go type name of output
}

// renderGoPackage adds the three files of one typed service package.
func renderGoPackage(module string, iface *schema.Interface, metadata []byte, files map[string][]byte) error {
	if _, err := schema.ParseVersion(iface.Version); err != nil {
		return sdkerrors.New(sdkerrors.KindVersion, "illegal version in generation").
			WithService(iface.Name).WithVersion(iface.Version).Wrap(err)
	}
	vm := tpl{
		Module:  module,
		Service: iface.Name,
		Version: iface.Version,
		Codec:   iface.Codec,
		Doc:     iface.Doc,
	}

	// Named types -> type declarations.
	nameByDef := map[string]string{}
	for i, nt := range iface.Types {
		goName := exported(nt.Name)
		nameByDef[nt.Name] = goName
		decl, err := goDecl(goName, nt.Type, nameByDef)
		if err != nil {
			return err.WithField("types." + nt.Name)
		}
		vm.Types = append(vm.Types, tplNamedType{GoName: goName, Doc: nt.Doc, Decl: decl})
		_ = i
	}

	// Operations input/output are usually inline anonymous objects; we
	// materialize them as named structs for an ergonomic client API.
	for _, op := range iface.Operations {
		inName := exported(op.Name) + "Input"
		outName := exported(op.Name) + "Output"
		// Inline object types become local anonymous struct declarations in
		// the operation-specific named types, appended to vm.Types.
		if err := appendAnon(&vm, inName, op.Input, nameByDef); err != nil {
			return err.WithOp(op.Name).WithField("input")
		}
		if err := appendAnon(&vm, outName, op.Output, nameByDef); err != nil {
			return err.WithOp(op.Name).WithField("output")
		}
		vm.Operations = append(vm.Operations, tplOperation{
			Name: exported(op.Name), Doc: op.Doc, Input: inName, Output: outName,
		})
	}

	sort.Slice(vm.Types, func(i, j int) bool { return vm.Types[i].GoName < vm.Types[j].GoName })

	clientSrc, err := execTemplate(clientTemplate, "client", vm)
	if err != nil {
		return sdkerrors.New(sdkerrors.KindGeneration, "cannot render client source").Wrap(err)
	}
	typesSrc, err := execTemplate(typesTemplate, "types", vm)
	if err != nil {
		return sdkerrors.New(sdkerrors.KindGeneration, "cannot render types source").Wrap(err)
	}

	pkgDir := filepath.Join(goSDKDir, "sdk", iface.Name, iface.Version)
	files[filepath.Join(pkgDir, iface.Name+".go")] = []byte(clientSrc)
	files[filepath.Join(pkgDir, "types.go")] = []byte(typesSrc)
	files[filepath.Join(pkgDir, "metadata.json")] = metadata
	return nil
}

func appendAnon(vm *tpl, goName string, t schema.Type, refs map[string]string) *sdkerrors.Error {
	decl, err := goDecl(goName, t, refs)
	if err != nil {
		return err
	}
	vm.Types = append(vm.Types, tplNamedType{GoName: goName, Decl: decl})
	return nil
}

func execTemplate(t *template.Template, name string, vm tpl) (string, error) {
	var b bytes.Buffer
	if err := t.ExecuteTemplate(&b, name, vm); err != nil {
		return "", err
	}
	return b.String(), nil
}

// goDecl renders one top-level type declaration.
func goDecl(goName string, t schema.Type, refs map[string]string) (string, *sdkerrors.Error) {
	switch t.Kind {
	case schema.TypeObject:
		var b strings.Builder
		b.WriteString("type " + goName + " struct {\n")
		for _, f := range t.Fields {
			ft, err := goType(f.Type, refs)
			if err != nil {
				return "", err
			}
			tag := "`json:\"" + f.Name
			if f.Optional {
				tag += ",omitempty"
			}
			tag += "\"`"
			b.WriteString("\t" + exported(f.Name) + " " + ft + " " + tag + "\n")
		}
		b.WriteString("}\n")
		return b.String(), nil
	case schema.TypeEnum:
		return "type " + goName + " string\n", nil
	case schema.TypeRef:
		base, ok := refs[t.RefName]
		if !ok {
			return "", sdkerrors.New(sdkerrors.KindInput, "unresolved ref during codegen").
				WithField(t.RefName)
		}
		return "type " + goName + " " + base + "\n", nil
	default:
		gt, err := goType(t, refs)
		if err != nil {
			return "", err
		}
		return "type " + goName + " " + gt + "\n", nil
	}
}

// goType renders a field/element Go type expression.
func goType(t schema.Type, refs map[string]string) (string, *sdkerrors.Error) {
	switch t.Kind {
	case schema.KindString:
		return "string", nil
	case schema.KindInteger:
		return "int64", nil
	case schema.KindNumber:
		return "float64", nil
	case schema.KindBoolean:
		return "bool", nil
	case schema.TypeRef:
		if n, ok := refs[t.RefName]; ok {
			return n, nil
		}
		return "", sdkerrors.New(sdkerrors.KindInput, "unresolved ref during codegen").
			WithField(t.RefName)
	case schema.TypeEnum:
		return "string", nil
	case schema.TypeArray:
		if t.Elem == nil {
			return "", sdkerrors.New(sdkerrors.KindInput, "array without element type")
		}
		e, err := goType(*t.Elem, refs)
		if err != nil {
			return "", err
		}
		return "[]" + e, nil
	case schema.TypeMap:
		if t.Elem == nil {
			return "", sdkerrors.New(sdkerrors.KindInput, "map without element type")
		}
		e, err := goType(*t.Elem, refs)
		if err != nil {
			return "", err
		}
		return "map[string]" + e, nil
	case schema.TypeObject:
		var b strings.Builder
		b.WriteString("struct {\n")
		for _, f := range t.Fields {
			ft, err := goType(f.Type, refs)
			if err != nil {
				return "", err
			}
			tag := "`json:\"" + f.Name
			if f.Optional {
				tag += ",omitempty"
			}
			tag += "\"`"
			b.WriteString("\t\t" + exported(f.Name) + " " + ft + " " + tag + "\n")
		}
		b.WriteString("\t}")
		return b.String(), nil
	default:
		return "", sdkerrors.Newf(sdkerrors.KindInput, "unknown type kind %q in codegen", t.Kind)
	}
}

// exported turns a snake_case / lowerCamel identifier into an exported
// Go identifier deterministically.
func exported(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' })
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	if b.Len() == 0 {
		return "X"
	}
	return b.String()
}
