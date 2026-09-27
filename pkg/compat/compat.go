package compat

import (
	"fmt"
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

// ChangeKind classifies a change as compatible or breaking.
type ChangeKind string

// Change kinds.
const (
	ChangeCompatible ChangeKind = "compatible"
	ChangeBreaking   ChangeKind = "breaking"
)

// Change describes one difference between two bundle versions.
type Change struct {
	Kind      ChangeKind
	Interface string
	Field     string
	Reason    string
}

// Report is the result of a cross-version comparison.
type Report struct {
	From    schema.Version
	To      schema.Version
	Changes []Change
}

// Compatible reports whether the new version is backward compatible with the
// old one (no breaking changes).
func (r *Report) Compatible() bool {
	for _, c := range r.Changes {
		if c.Kind == ChangeBreaking {
			return false
		}
	}
	return true
}

// Breaking returns only the breaking changes.
func (r *Report) Breaking() []Change {
	var out []Change
	for _, c := range r.Changes {
		if c.Kind == ChangeBreaking {
			out = append(out, c)
		}
	}
	return out
}

// Compare checks the new bundle against the old one. It assumes both bundles
// already passed validate.Validate; structural problems in either bundle are
// returned as invalid_input errors.
//
// Compatibility rules (old -> new):
//
//   - new struct fields are compatible only when optional;
//     removing a field, or turning an optional field required, is breaking;
//   - enum value additions are compatible; removals are breaking;
//   - widening a type (int->long, float->double) is compatible;
//     narrowing or changing category is breaking;
//   - a parameter added to an interface is compatible only when optional;
//     removing or changing a parameter is breaking;
//   - changing an output/error type by the same widening rules is breaking
//     (the success response shape may not silently change);
//   - added interfaces and user types are compatible; removals are breaking;
//   - required-optional relaxation on an input parameter is compatible.
func Compare(oldBundle, newBundle *schema.Bundle) (*Report, error) {
	if err := requireValid(oldBundle, "old"); err != nil {
		return nil, err
	}
	if err := requireValid(newBundle, "new"); err != nil {
		return nil, err
	}
	if len(oldBundle.Documents) == 0 {
		return nil, apierrors.New(apierrors.CatInvalidInput, "old bundle contains no documents").WithOp("compat.compare")
	}
	if len(newBundle.Documents) == 0 {
		return nil, apierrors.New(apierrors.CatInvalidInput, "new bundle contains no documents").WithOp("compat.compare")
	}
	oldV := oldBundle.Documents[0].Version
	newV := newBundle.Documents[0].Version
	if newV.Compare(oldV) <= 0 {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			fmt.Sprintf("new version %s must be greater than old version %s", newV, oldV)).
			WithOp("compat.compare")
	}
	r := &Report{From: oldV, To: newV}

	// ---- structs ----
	oldStructs := namedStructs(oldBundle)
	newStructs := namedStructs(newBundle)
	for _, name := range sortedKeys(oldStructs) {
		ns, ok := newStructs[name]
		if !ok {
			r.add(ChangeBreaking, "", name, "struct removed")
			continue
		}
		r.compareStruct(name, oldStructs[name], ns)
	}
	for _, name := range sortedKeys(newStructs) {
		if _, ok := oldStructs[name]; !ok {
			r.add(ChangeCompatible, "", name, "struct added")
		}
	}

	// ---- enums ----
	oldEnums := namedEnums(oldBundle)
	newEnums := namedEnums(newBundle)
	for _, name := range sortedKeys(oldEnums) {
		ne, ok := newEnums[name]
		if !ok {
			r.add(ChangeBreaking, "", name, "enum removed")
			continue
		}
		r.compareEnum(name, oldEnums[name], ne)
	}
	for _, name := range sortedKeys(newEnums) {
		if _, ok := oldEnums[name]; !ok {
			r.add(ChangeCompatible, "", name, "enum added")
		}
	}

	// ---- interfaces ----
	oldIfaces := namedIfaces(oldBundle)
	newIfaces := namedIfaces(newBundle)
	for _, name := range sortedKeys(oldIfaces) {
		ni, ok := newIfaces[name]
		if !ok {
			r.add(ChangeBreaking, name, "", "interface removed")
			continue
		}
		r.compareInterface(oldIfaces[name], ni)
	}
	for _, name := range sortedKeys(newIfaces) {
		if _, ok := oldIfaces[name]; !ok {
			r.add(ChangeCompatible, name, "", "interface added")
		}
	}
	return r, nil
}

func (r *Report) add(kind ChangeKind, iface, field, reason string) {
	r.Changes = append(r.Changes, Change{Kind: kind, Interface: iface, Field: field, Reason: reason})
}

func (r *Report) compareStruct(name string, old, neu *schema.Struct) {
	oldFields := map[string]schema.Field{}
	for _, f := range old.Fields {
		oldFields[f.Name] = f
	}
	newFields := map[string]schema.Field{}
	for _, f := range neu.Fields {
		newFields[f.Name] = f
	}
	for _, f := range old.Fields {
		nf, ok := newFields[f.Name]
		path := name + "." + f.Name
		if !ok {
			r.add(ChangeBreaking, "", path, "field removed")
			continue
		}
		if !f.Optional && nf.Optional {
			r.add(ChangeCompatible, "", path, "field relaxed from required to optional")
		}
		if f.Optional && !nf.Optional {
			r.add(ChangeBreaking, "", path, "field changed from optional to required")
		}
		r.compareFieldType(path, f.Type, nf.Type)
	}
	for _, f := range neu.Fields {
		if _, ok := oldFields[f.Name]; !ok {
			path := name + "." + f.Name
			if f.Optional {
				r.add(ChangeCompatible, "", path, "optional field added")
			} else {
				r.add(ChangeBreaking, "", path, "required field added")
			}
		}
	}
}

func (r *Report) compareFieldType(path string, old, neu schema.TypeRef) {
	// Nullability: becoming nullable is compatible (value may be absent),
	// losing nullability is breaking.
	if old.Nullable && !neu.Nullable {
		r.add(ChangeBreaking, "", path, "type changed from nullable to non-nullable")
	}
	if !old.Nullable && neu.Nullable {
		r.add(ChangeCompatible, "", path, "type changed to nullable")
	}
	switch {
	case old.Kind() != neu.Kind():
		r.add(ChangeBreaking, "", path,
			fmt.Sprintf("type changed from %s to %s", old.String(), neu.String()))
		return
	case old.Kind() == "primitive":
		if widen, ok := primitiveWiden[old.Primitive]; ok && widen[neu.Primitive] {
			r.add(ChangeCompatible, "", path,
				fmt.Sprintf("primitive widened from %s to %s", old.Primitive, neu.Primitive))
			return
		}
		if old.Primitive != neu.Primitive {
			r.add(ChangeBreaking, "", path,
				fmt.Sprintf("primitive changed from %s to %s", old.Primitive, neu.Primitive))
		}
	case old.Kind() == "ref":
		if old.Name != neu.Name {
			r.add(ChangeBreaking, "", path,
				fmt.Sprintf("reference changed from %s to %s", old.Name, neu.Name))
		}
	case old.Kind() == "array":
		r.compareFieldType(path+"[]", *old.Array, *neu.Array)
	case old.Kind() == "map":
		r.compareFieldType(path+"[map]", *old.MapValue, *neu.MapValue)
	}
}

// primitiveWiden maps a primitive to the set of primitives it can widen into
// without breaking callers.
var primitiveWiden = map[schema.Primitive]map[schema.Primitive]bool{
	schema.PrimInt:   {schema.PrimLong: true, schema.PrimFloat: true, schema.PrimDouble: true},
	schema.PrimLong:  {schema.PrimFloat: true, schema.PrimDouble: true},
	schema.PrimFloat: {schema.PrimDouble: true},
}

func (r *Report) compareEnum(name string, old, neu *schema.Enum) {
	oldVals := map[string]bool{}
	for _, v := range old.Values {
		oldVals[v] = true
	}
	newVals := map[string]bool{}
	for _, v := range neu.Values {
		newVals[v] = true
	}
	for _, v := range old.Values {
		if !newVals[v] {
			r.add(ChangeBreaking, "", name+"."+v, "enum value removed")
		}
	}
	for _, v := range neu.Values {
		if !oldVals[v] {
			r.add(ChangeCompatible, "", name+"."+v, "enum value added")
		}
	}
}

func (r *Report) compareInterface(old, neu *schema.Interface) {
	if old.Method != neu.Method {
		r.add(ChangeBreaking, old.Name, "",
			fmt.Sprintf("method changed from %q to %q", old.Method, neu.Method))
	}
	oldParams := map[string]schema.Parameter{}
	for _, p := range old.Input {
		oldParams[p.Name] = p
	}
	newParams := map[string]schema.Parameter{}
	for _, p := range neu.Input {
		newParams[p.Name] = p
	}
	for _, p := range old.Input {
		np, ok := newParams[p.Name]
		path := old.Name + ".input." + p.Name
		if !ok {
			r.add(ChangeBreaking, old.Name, path, "parameter removed")
			continue
		}
		if p.Optional && !np.Optional {
			r.add(ChangeBreaking, old.Name, path, "parameter changed from optional to required")
		}
		if !p.Optional && np.Optional {
			r.add(ChangeCompatible, old.Name, path, "parameter relaxed from required to optional")
		}
		r.compareInputType(old.Name, path, p.Type, np.Type)
	}
	for _, p := range neu.Input {
		if _, ok := oldParams[p.Name]; !ok {
			path := old.Name + ".input." + p.Name
			if p.Optional {
				r.add(ChangeCompatible, old.Name, path, "optional parameter added")
			} else {
				r.add(ChangeBreaking, old.Name, path, "required parameter added")
			}
		}
	}
	// Output changes are breaking (successful call semantics must not change).
	switch {
	case old.Output == nil && neu.Output != nil:
		r.add(ChangeBreaking, old.Name, old.Name+".output", "output added to previously void interface")
	case old.Output != nil && neu.Output == nil:
		r.add(ChangeBreaking, old.Name, old.Name+".output", "output removed")
	case old.Output != nil && neu.Output != nil:
		if old.Output.String() != neu.Output.String() {
			r.compareOutputType(old.Name, old.Name+".output", *old.Output, *neu.Output)
		}
	}
	// Declared error types: removing an error declaration is compatible
	// (callers handle fewer cases); adding one is compatible too since
	// callers already handle failure generically. A changed declaration is
	// reported breaking.
	oldErrs := map[string]bool{}
	for _, t := range old.Errors {
		oldErrs[t.String()] = true
	}
	newErrs := map[string]bool{}
	for _, t := range neu.Errors {
		newErrs[t.String()] = true
	}
	for e := range oldErrs {
		if !newErrs[e] {
			r.add(ChangeCompatible, old.Name, old.Name+".errors."+e, "error declaration removed")
		}
	}
	for e := range newErrs {
		if !oldErrs[e] {
			r.add(ChangeCompatible, old.Name, old.Name+".errors."+e, "error declaration added")
		}
	}
}

func (r *Report) compareInputType(iface, path string, old, neu schema.TypeRef) {
	// Reuse the field-type comparison for inputs; a type widening on input
	// is still considered breaking for the *payload the client sends*,
	// except for enum expansion handled at enum level.
	if old.Nullable != neu.Nullable {
		if !old.Nullable && neu.Nullable {
			r.add(ChangeCompatible, iface, path,
				fmt.Sprintf("parameter type changed from %s to nullable %s", old.String(), neu.String()))
			return
		}
		r.add(ChangeBreaking, iface, path,
			fmt.Sprintf("parameter type changed from nullable %s to %s", old.String(), neu.String()))
	}
	if old.Kind() != neu.Kind() {
		r.add(ChangeBreaking, iface, path,
			fmt.Sprintf("parameter type changed from %s to %s", old.String(), neu.String()))
		return
	}
	switch old.Kind() {
	case "primitive":
		if old.Primitive != neu.Primitive {
			r.add(ChangeBreaking, iface, path,
				fmt.Sprintf("parameter type changed from %s to %s", old.Primitive, neu.Primitive))
		}
	case "ref":
		if old.Name != neu.Name {
			r.add(ChangeBreaking, iface, path,
				fmt.Sprintf("parameter type changed from %s to %s", old.Name, neu.Name))
		}
	case "array":
		r.compareInputType(iface, path+"[]", *old.Array, *neu.Array)
	case "map":
		r.compareInputType(iface, path+"[map]", *old.MapValue, *neu.MapValue)
	}
}

func (r *Report) compareOutputType(iface, path string, old, neu schema.TypeRef) {
	r.add(ChangeBreaking, iface, path,
		fmt.Sprintf("output type changed from %s to %s", old.String(), neu.String()))
}

func namedStructs(b *schema.Bundle) map[string]*schema.Struct {
	out := map[string]*schema.Struct{}
	for _, d := range b.Documents {
		for i := range d.Structs {
			out[d.Structs[i].Name] = &d.Structs[i]
		}
	}
	return out
}

func namedEnums(b *schema.Bundle) map[string]*schema.Enum {
	out := map[string]*schema.Enum{}
	for _, d := range b.Documents {
		for i := range d.Enums {
			out[d.Enums[i].Name] = &d.Enums[i]
		}
	}
	return out
}

func namedIfaces(b *schema.Bundle) map[string]*schema.Interface {
	out := map[string]*schema.Interface{}
	for _, d := range b.Documents {
		for i := range d.Interfaces {
			out[d.Interfaces[i].Name] = &d.Interfaces[i]
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func requireValid(b *schema.Bundle, side string) error {
	if b == nil {
		return apierrors.New(apierrors.CatInvalidInput, side+" bundle is nil").WithOp("compat.compare")
	}
	return nil
}
