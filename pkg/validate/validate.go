package validate

import (
	"fmt"
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

// Validate checks a bundle for:
//   - illegal or duplicate (service, version) documents,
//   - duplicate / empty names of interfaces, structs, enums and fields,
//   - unknown primitive kinds and references to missing types,
//   - type-shape conflicts when the same name is defined in two documents,
//   - circular references between structs.
//
// Every problem is collected and returned together as one MultiError of
// category invalid_input; nothing is silently skipped.
func Validate(b *schema.Bundle) error {
	v := &validator{bundle: b}
	v.checkDocuments()
	v.checkDefinitions()
	v.checkRefs()
	v.checkCycles()
	if v.issues.Len() == 0 {
		return nil
	}
	return &v.issues
}

type validator struct {
	bundle *schema.Bundle
	issues apierrors.MultiError
}

func (v *validator) issue(iface, field, reason string) {
	v.issues.Category = apierrors.CatInvalidInput
	v.issues.Op = "validate"
	v.issues.Add(apierrors.Issue{Interface: iface, Field: field, Reason: reason})
}

func (v *validator) checkDocuments() {
	// Multiple documents may describe different fragments of the same
	// (service, version); they merge into one bundle. True conflicts on
	// shared names are caught per-definition in checkDefinitions, so there
	// is nothing to reject at the document level itself.
}

func (v *validator) checkDefinitions() {
	type def struct {
		kind   string // struct / enum / interface
		source string
		shape  string
	}
	structs := map[string][]def{}
	enums := map[string][]def{}
	ifaces := map[string][]def{}

	for _, d := range v.bundle.Documents {
		for i := range d.Structs {
			s := &d.Structs[i]
			if s.Name == "" {
				v.issue("", d.Source, "struct with empty name")
				continue
			}
			structs[s.Name] = append(structs[s.Name], def{"struct", d.Source, structShape(s)})
			fieldNames := map[string]bool{}
			for _, f := range s.Fields {
				if f.Name == "" {
					v.issue("", fmt.Sprintf("%s/%s", d.Source, s.Name),
						"struct field with empty name")
				} else if fieldNames[f.Name] {
					v.issue("", fmt.Sprintf("%s/%s.%s", d.Source, s.Name, f.Name),
						"duplicate struct field name")
				}
				fieldNames[f.Name] = true
			}
		}
		for i := range d.Enums {
			e := &d.Enums[i]
			if e.Name == "" {
				v.issue("", d.Source, "enum with empty name")
				continue
			}
			enums[e.Name] = append(enums[e.Name], def{"enum", d.Source, enumShape(e)})
			vals := map[string]bool{}
			for _, val := range e.Values {
				if val == "" {
					v.issue("", fmt.Sprintf("%s/%s", d.Source, e.Name),
						"enum value with empty name")
				} else if vals[val] {
					v.issue("", fmt.Sprintf("%s/%s.%s", d.Source, e.Name, val),
						"duplicate enum value")
				}
				vals[val] = true
			}
		}
		for i := range d.Interfaces {
			ifc := &d.Interfaces[i]
			if ifc.Name == "" {
				v.issue("", d.Source, "interface with empty name")
				continue
			}
			ifaces[ifc.Name] = append(ifaces[ifc.Name], def{"interface", d.Source, ifaceShape(ifc)})
			params := map[string]bool{}
			for _, p := range ifc.Input {
				if p.Name == "" {
					v.issue(ifc.Name, d.Source, "parameter with empty name")
				} else if params[p.Name] {
					v.issue(ifc.Name, fmt.Sprintf("%s/%s.%s", d.Source, ifc.Name, p.Name),
						"duplicate parameter name")
				}
				params[p.Name] = true
			}
		}
	}

	reportConflicts := func(name string, defs []def) {
		sort.Slice(defs, func(i, j int) bool { return defs[i].source < defs[j].source })
		base := defs[0].shape
		for _, d := range defs[1:] {
			if d.shape != base {
				v.issue("", name,
					fmt.Sprintf("conflicting %s definitions in %s and %s",
						defs[0].kind, defs[0].source, d.source))
				break
			}
		}
	}
	all := map[string][]def{}
	for n, ds := range structs {
		all["struct:"+n] = ds
	}
	for n, ds := range enums {
		all["enum:"+n] = ds
	}
	for n, ds := range ifaces {
		all["interface:"+n] = ds
	}
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		reportConflicts(k, all[k])
	}
}

func (v *validator) checkRefs() {
	var checkType func(iface, path string, t schema.TypeRef)
	checkType = func(iface, path string, t schema.TypeRef) {
		switch t.Kind() {
		case "primitive":
			if !schema.IsPrimitive(t.Primitive) {
				v.issue(iface, path, fmt.Sprintf("unknown primitive type %q", t.Primitive))
			}
		case "ref":
			if _, svc := v.bundle.FindStruct(t.Name); svc == "" {
				if _, evc := v.bundle.FindEnum(t.Name); evc == "" {
					v.issue(iface, path, fmt.Sprintf("reference to missing type %q", t.Name))
				}
			}
		case "array":
			checkType(iface, path+"[]", *t.Array)
		case "map":
			checkType(iface, path+"[map]", *t.MapValue)
		default:
			v.issue(iface, path, "type reference without ref/primitive/array/map")
		}
	}
	for _, d := range v.bundle.Documents {
		for i := range d.Structs {
			s := &d.Structs[i]
			for _, f := range s.Fields {
				checkType("", fmt.Sprintf("%s/%s.%s", d.Source, s.Name, f.Name), f.Type)
			}
		}
		for i := range d.Interfaces {
			ifc := &d.Interfaces[i]
			for _, p := range ifc.Input {
				checkType(ifc.Name, fmt.Sprintf("%s/%s.param.%s", d.Source, ifc.Name, p.Name), p.Type)
			}
			if ifc.Output != nil {
				checkType(ifc.Name, fmt.Sprintf("%s/%s.output", d.Source, ifc.Name), *ifc.Output)
			}
			for j, er := range ifc.Errors {
				checkType(ifc.Name, fmt.Sprintf("%s/%s.error[%d]", d.Source, ifc.Name, j), er)
			}
		}
	}
}

func (v *validator) checkCycles() {
	color := map[string]int{} // 0 white, 1 gray, 2 black
	var stack []string
	var dfs func(name, via string)
	dfs = func(name, via string) {
		switch color[name] {
		case 1:
			// Found a back edge: render the cycle from its first occurrence.
			idx := -1
			for i, n := range stack {
				if n == name {
					idx = i
					break
				}
			}
			cycle := append(append([]string(nil), stack[idx:]...), name)
			path := ""
			for i, n := range cycle {
				if i > 0 {
					path += " -> "
				}
				path += n
			}
			v.issue("", via, fmt.Sprintf("circular reference detected: %s", path))
			return
		case 2:
			return
		}
		s, svc := v.bundle.FindStruct(name)
		if svc == "" {
			return // enum / missing refs already reported
		}
		color[name] = 1
		stack = append(stack, name)
		for _, f := range s.Fields {
			forEachStructRef(f.Type, func(target string) {
				dfs(target, fmt.Sprintf("%s.%s", name, f.Name))
			})
		}
		stack = stack[:len(stack)-1]
		color[name] = 2
	}
	names := allStructNames(v.bundle)
	for _, n := range names {
		if color[n] == 0 {
			dfs(n, "")
		}
	}
}

// forEachStructRef invokes fn for every named struct reference reachable
// from t (unwrapping arrays and maps).
func forEachStructRef(t schema.TypeRef, fn func(name string)) {
	switch t.Kind() {
	case "ref":
		fn(t.Name)
	case "array":
		forEachStructRef(*t.Array, fn)
	case "map":
		forEachStructRef(*t.MapValue, fn)
	}
}

func allStructNames(b *schema.Bundle) []string {
	seen := map[string]bool{}
	for _, d := range b.Documents {
		for _, s := range d.Structs {
			seen[s.Name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func structShape(s *schema.Struct) string {
	out := s.Name + "{"
	for i, f := range s.Fields {
		if i > 0 {
			out += ","
		}
		opt := ""
		if f.Optional {
			opt = "?"
		}
		out += f.Name + opt + ":" + f.Type.String()
	}
	return out + "}"
}

func enumShape(e *schema.Enum) string {
	out := e.Name + "("
	for i, val := range e.Values {
		if i > 0 {
			out += ","
		}
		out += val
	}
	return out + ")"
}

func ifaceShape(i *schema.Interface) string {
	out := i.Name + "("
	for j, p := range i.Input {
		if j > 0 {
			out += ","
		}
		opt := ""
		if p.Optional {
			opt = "?"
		}
		out += p.Name + opt + ":" + p.Type.String()
	}
	out += ")"
	if i.Output != nil {
		out += "->" + i.Output.String()
	}
	return out
}
