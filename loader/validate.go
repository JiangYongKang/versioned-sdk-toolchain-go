package loader

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// convertAndValidate converts the DTO to the canonical model and runs
// every structural check. Nothing invalid is silently accepted.
func convertAndValidate(d *fileDTO, source string) (*schema.Interface, error) {
	svc := strings.TrimSpace(d.Name)
	if svc == "" {
		return nil, sdkerrors.New(sdkerrors.KindInput, "missing interface name").
			WithField("name").Wrap(fmt.Errorf("source=%s", source))
	}
	if !identRe.MatchString(svc) {
		return nil, sdkerrors.New(sdkerrors.KindInput,
			"invalid interface name: must match [A-Za-z_][A-Za-z0-9_]*").
			WithService(svc).WithField("name").Wrap(fmt.Errorf("source=%s", source))
	}

	ver, err := schema.ParseVersion(d.Version)
	if err != nil {
		return nil, sdkerrors.New(sdkerrors.KindInput, "illegal version").
			WithService(svc).WithVersion(d.Version).
			Wrap(fmt.Errorf("source=%s: %w", source, err))
	}

	codec := strings.TrimSpace(d.Codec)
	if codec == "" {
		codec = "json"
	}
	if !identRe.MatchString(codec) {
		return nil, sdkerrors.New(sdkerrors.KindInput,
			"invalid codec name: must match [A-Za-z_][A-Za-z0-9_]*").
			WithService(svc).WithVersion(ver.String()).WithField("codec").
			Wrap(fmt.Errorf("source=%s", source))
	}

	if len(d.Operations) == 0 {
		return nil, sdkerrors.New(sdkerrors.KindInput, "interface declares no operations").
			WithService(svc).WithVersion(ver.String())
	}

	iface := &schema.Interface{
		Name:    svc,
		Version: ver.String(),
		Codec:   codec,
		Doc:     d.Doc,
	}

	// Named types first: names must be unique and valid.
	typeNames := map[string]bool{}
	for _, nt := range d.Types {
		name := strings.TrimSpace(nt.Name)
		if name == "" || !identRe.MatchString(name) {
			return nil, sdkerrors.New(sdkerrors.KindInput,
				"invalid type name: must be a non-empty identifier").
				WithService(svc).WithVersion(iface.Version).
				WithField("types." + nt.Name).Wrap(fmt.Errorf("source=%s", source))
		}
		if typeNames[name] {
			return nil, sdkerrors.New(sdkerrors.KindInput,
				"duplicate named type within one interface").
				WithService(svc).WithVersion(iface.Version).
				WithField("types." + name).Wrap(fmt.Errorf("source=%s", source))
		}
		typeNames[name] = true
		t, err := convertType(nt.Type)
		if err != nil {
			return nil, annotate(err, svc, iface.Version, "types."+name, source)
		}
		iface.Types = append(iface.Types, schema.NamedType{Name: name, Doc: nt.Doc, Type: t})
	}

	// Operations: names unique and valid; inputs/outputs converted.
	opNames := map[string]bool{}
	for _, op := range d.Operations {
		name := strings.TrimSpace(op.Name)
		if name == "" || !identRe.MatchString(name) {
			return nil, sdkerrors.New(sdkerrors.KindInput,
				"invalid operation name: must be a non-empty identifier").
				WithService(svc).WithVersion(iface.Version).
				WithField("operations").Wrap(fmt.Errorf("source=%s", source))
		}
		if opNames[name] {
			return nil, sdkerrors.New(sdkerrors.KindInput,
				"duplicate operation name within one interface").
				WithService(svc).WithVersion(iface.Version).
				WithField("operations." + name).Wrap(fmt.Errorf("source=%s", source))
		}
		opNames[name] = true
		in, err := convertType(op.Input)
		if err != nil {
			return nil, annotate(err, svc, iface.Version, "operations."+name+".input", source)
		}
		out, err := convertType(op.Output)
		if err != nil {
			return nil, annotate(err, svc, iface.Version, "operations."+name+".output", source)
		}
		iface.Operations = append(iface.Operations, schema.Operation{
			Name: name, Doc: op.Doc, Input: in, Output: out,
		})
	}

	if err := validateRefs(iface, typeNames, source); err != nil {
		return nil, err
	}
	if err := detectCycles(iface, source); err != nil {
		return nil, err
	}
	return iface, nil
}

func annotate(err error, svc, ver, field, source string) error {
	se := sdkerrors.AsError(err)
	if se == nil {
		return sdkerrors.New(sdkerrors.KindInput, err.Error()).
			WithService(svc).WithVersion(ver).WithField(field).
			Wrap(fmt.Errorf("source=%s", source))
	}
	se.WithService(svc).WithVersion(ver)
	if se.Field == "" {
		se.WithField(field)
	} else {
		se.WithField(field + "." + se.Field)
	}
	if se.Cause == nil {
		se.Wrap(fmt.Errorf("source=%s", source))
	}
	return se
}

// convertType recursively converts a DTO type, enforcing shape rules
// (enum requires values; array/map requires elem; object fields unique).
func convertType(d typeDTO) (schema.Type, error) {
	kind := schema.TypeKind(strings.TrimSpace(d.Kind))
	t := schema.Type{Kind: kind}
	switch kind {
	case schema.KindString, schema.KindInteger, schema.KindNumber, schema.KindBoolean:
		return t, nil
	case schema.TypeRef:
		if strings.TrimSpace(d.Ref) == "" {
			return t, sdkerrors.New(sdkerrors.KindInput, "ref type is missing target name")
		}
		t.RefName = d.Ref
		return t, nil
	case schema.TypeEnum:
		if len(d.Values) == 0 {
			return t, sdkerrors.New(sdkerrors.KindInput, "enum type declares no values")
		}
		seen := map[string]bool{}
		for _, v := range d.Values {
			if v == "" {
				return t, sdkerrors.New(sdkerrors.KindInput, "enum contains an empty value")
			}
			if seen[v] {
				return t, sdkerrors.New(sdkerrors.KindInput,
					"enum contains a duplicate value").WithField(fmt.Sprintf("values.%s", v))
			}
			seen[v] = true
		}
		t.EnumVals = append([]string(nil), d.Values...)
		return t, nil
	case schema.TypeArray, schema.TypeMap:
		if d.Elem == nil {
			return t, sdkerrors.New(sdkerrors.KindInput,
				string(kind)+" type is missing element type").WithField("elem")
		}
		elem, err := convertType(*d.Elem)
		if err != nil {
			return t, err
		}
		t.Elem = &elem
		return t, nil
	case schema.TypeObject:
		if len(d.Fields) == 0 {
			return t, sdkerrors.New(sdkerrors.KindInput, "object type declares no fields")
		}
		names := map[string]bool{}
		for _, f := range d.Fields {
			if strings.TrimSpace(f.Name) == "" {
				return t, sdkerrors.New(sdkerrors.KindInput, "object field has an empty name")
			}
			if names[f.Name] {
				return t, sdkerrors.New(sdkerrors.KindInput,
					"object contains a duplicate field name").WithField("fields." + f.Name)
			}
			names[f.Name] = true
			ft, err := convertType(f.Type)
			if err != nil {
				return t, err
			}
			t.Fields = append(t.Fields, schema.Field{
				Name: f.Name, Type: ft, Optional: f.Optional, Doc: f.Doc,
			})
		}
		return t, nil
	default:
		return t, sdkerrors.Newf(sdkerrors.KindInput, "unknown type kind %q", d.Kind)
	}
}

// validateRefs walks every reachable type and rejects unresolved refs.
func validateRefs(iface *schema.Interface, names map[string]bool, source string) error {
	for _, nt := range iface.Types {
		if err := walkType(nt.Type, names, "types."+nt.Name, iface, source); err != nil {
			return err
		}
	}
	for _, op := range iface.Operations {
		if err := walkType(op.Input, names, "operations."+op.Name+".input", iface, source); err != nil {
			return err
		}
		if err := walkType(op.Output, names, "operations."+op.Name+".output", iface, source); err != nil {
			return err
		}
	}
	return nil
}

func walkType(t schema.Type, names map[string]bool, path string, iface *schema.Interface, source string) error {
	switch t.Kind {
	case schema.TypeRef:
		if !names[t.RefName] {
			return sdkerrors.New(sdkerrors.KindInput,
				"missing reference: no named type declared with that name").
				WithService(iface.Name).WithVersion(iface.Version).
				WithField(path).Wrap(fmt.Errorf("ref=%s, source=%s", t.RefName, source))
		}
	case schema.TypeArray, schema.TypeMap:
		if t.Elem != nil {
			return walkType(*t.Elem, names, path+".elem", iface, source)
		}
	case schema.TypeObject:
		for _, f := range t.Fields {
			if err := walkType(f.Type, names, path+".fields."+f.Name, iface, source); err != nil {
				return err
			}
		}
	}
	return nil
}

// detectCycles finds cycles in the named-type reference graph. A cycle
// through a struct field is rejected (no lazy/pointer representation is
// generated); the diagnostic lists the full reference chain.
func detectCycles(iface *schema.Interface, source string) error {
	defs := map[string]schema.Type{}
	adj := map[string][]string{}
	names := make([]string, 0, len(iface.Types))
	for _, nt := range iface.Types {
		defs[nt.Name] = nt.Type
		names = append(names, nt.Name)
	}
	for name, t := range defs {
		adj[name] = collectRefs(t, nil)
	}
	sort.Strings(names)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string

	var dfs func(n string) error
	dfs = func(n string) error {
		color[n] = gray
		stack = append(stack, n)
		next := append([]string(nil), adj[n]...)
		sort.Strings(next)
		for _, m := range next {
			switch color[m] {
			case gray:
				// Build the cycle slice from where m first appears.
				idx := 0
				for i, x := range stack {
					if x == m {
						idx = i
						break
					}
				}
				chain := append(append([]string(nil), stack[idx:]...), m)
				return sdkerrors.New(sdkerrors.KindInput,
					"circular reference detected between named types").
					WithService(iface.Name).WithVersion(iface.Version).
					WithField("types." + strings.Join(chain, " -> ")).
					Wrap(fmt.Errorf("source=%s", source))
			case white:
				if err := dfs(m); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		return nil
	}

	for _, n := range names {
		if color[n] == white {
			if err := dfs(n); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectRefs(t schema.Type, out []string) []string {
	switch t.Kind {
	case schema.TypeRef:
		out = append(out, t.RefName)
	case schema.TypeArray, schema.TypeMap:
		if t.Elem != nil {
			out = collectRefs(*t.Elem, out)
		}
	case schema.TypeObject:
		for _, f := range t.Fields {
			out = collectRefs(f.Type, out)
		}
	}
	return out
}
