// Package compat decides whether interface changes across versions are
// backward compatible and refuses breaking changes with precise diagnostics.
//
// Rules (old -> new):
//
// Compatibility (SDK regenerates successfully):
//   - adding a new optional field to an object
//   - adding a new enum value (enumeration expansion)
//   - adding a new named type
//   - adding a new operation
//   - adding documentation
//
// Breaking (generation is refused):
//   - removing a field / operation / named type
//   - changing a field's type shape (primitive kind, ref target,
//     array/map structure, enum membership semantics)
//   - removing an enum value
//   - turning a required field optional is breaking on inputs (callers may
//     omit it now, but older servers reject); turning an optional field
//     required is breaking on outputs and also on inputs for payloads
//     built by older SDKs. The toolchain conservatively treats ANY
//     required/optional change as breaking except adding a brand new
//     field as optional.
package compat

import (
	"fmt"
	"sort"
	"strings"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
)

// ChangeKind classifies a single detected difference.
type ChangeKind string

// Supported change classifications.
const (
	ChangeCompatible ChangeKind = "compatible"
	ChangeBreaking   ChangeKind = "breaking"
)

// Change describes one difference between two versions.
type Change struct {
	Kind    ChangeKind
	Service string
	FromVer string
	ToVer   string
	Op      string
	Field   string
	Detail  string
}

// Report is the result of comparing two versions.
type Report struct {
	Service string
	FromVer string
	ToVer   string
	Changes []Change
}

// Compatible reports whether no breaking changes were found.
func (r *Report) Compatible() bool {
	for _, c := range r.Changes {
		if c.Kind == ChangeBreaking {
			return false
		}
	}
	return true
}

// BreakingChanges returns only the breaking changes in a report.
func (r *Report) BreakingChanges() []Change {
	var out []Change
	for _, c := range r.Changes {
		if c.Kind == ChangeBreaking {
			out = append(out, c)
		}
	}
	return out
}

func (r *Report) add(kind ChangeKind, op, field, detail string) {
	r.Changes = append(r.Changes, Change{
		Kind: kind, Service: r.Service, FromVer: r.FromVer, ToVer: r.ToVer,
		Op: op, Field: field, Detail: detail,
	})
}

// Compare checks next against prev and records every change in
// deterministic order. prev and next must describe the same service.
func Compare(prev, next *schema.Interface) (*Report, error) {
	if prev == nil || next == nil {
		return nil, fmt.Errorf("compat.Compare: nil interface")
	}
	if prev.Name != next.Name {
		return nil, fmt.Errorf("compat.Compare: service mismatch %q vs %q", prev.Name, next.Name)
	}
	r := &Report{Service: next.Name, FromVer: prev.Version, ToVer: next.Version}

	compareNamedTypes(r, prev, next)
	compareOperations(r, prev, next)
	sortChanges(r.Changes)
	return r, nil
}

func sortChanges(cs []Change) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		if a.Op != b.Op {
			return a.Op < b.Op
		}
		return a.Detail < b.Detail
	})
}

func namedMap(iface *schema.Interface) map[string]schema.NamedType {
	m := make(map[string]schema.NamedType, len(iface.Types))
	for _, nt := range iface.Types {
		m[nt.Name] = nt
	}
	return m
}

func compareNamedTypes(r *Report, prev, next *schema.Interface) {
	old := namedMap(prev)
	new := namedMap(next)
	names := make([]string, 0, len(new)+len(old))
	seen := map[string]bool{}
	for n := range old {
		names = append(names, n)
		seen[n] = true
	}
	for n := range new {
		if !seen[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		o, inOld := old[n]
		nu, inNew := new[n]
		path := "types." + n
		switch {
		case inOld && !inNew:
			r.add(ChangeBreaking, "", path, "named type removed; all references and usages become invalid")
		case !inOld && inNew:
			r.add(ChangeCompatible, "", path, "new named type added")
		default:
			diffTypes(r, "", path, o.Type, nu.Type)
		}
	}
}

func compareOperations(r *Report, prev, next *schema.Interface) {
	old := map[string]schema.Operation{}
	for _, op := range prev.Operations {
		old[op.Name] = op
	}
	new := map[string]schema.Operation{}
	for _, op := range next.Operations {
		new[op.Name] = op
	}
	names := make([]string, 0, len(new)+len(old))
	seen := map[string]bool{}
	for n := range old {
		names = append(names, n)
		seen[n] = true
	}
	for n := range new {
		if !seen[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		o, inOld := old[n]
		nu, inNew := new[n]
		switch {
		case inOld && !inNew:
			r.add(ChangeBreaking, n, "operations."+n, "operation removed; callers invoking it will fail")
		case !inOld && inNew:
			r.add(ChangeCompatible, n, "operations."+n, "new operation added")
		default:
			diffTypes(r, n, "operations."+n+".input", o.Input, nu.Input)
			diffTypes(r, n, "operations."+n+".output", o.Output, nu.Output)
		}
	}
}

// diffTypes records structural differences. Ref targets are compared by
// name; two structurally equal anonymous types are equal.
func diffTypes(r *Report, op, path string, oldT, newT schema.Type) {
	if oldT.Kind != newT.Kind {
		r.add(ChangeBreaking, op, path, fmt.Sprintf(
			"type kind changed: %s -> %s", oldT.Kind, newT.Kind))
		return
	}
	switch oldT.Kind {
	case schema.KindString, schema.KindInteger, schema.KindNumber, schema.KindBoolean:
		return
	case schema.TypeRef:
		if oldT.RefName != newT.RefName {
			r.add(ChangeBreaking, op, path, fmt.Sprintf(
				"reference target changed: %s -> %s", oldT.RefName, newT.RefName))
		}
	case schema.TypeEnum:
		diffEnums(r, op, path, oldT.EnumVals, newT.EnumVals)
	case schema.TypeArray, schema.TypeMap:
		if (oldT.Elem == nil) != (newT.Elem == nil) {
			r.add(ChangeBreaking, op, path+".elem", "element type presence changed")
			return
		}
		if oldT.Elem != nil {
			diffTypes(r, op, path+".elem", *oldT.Elem, *newT.Elem)
		}
	case schema.TypeObject:
		diffFields(r, op, path, oldT.Fields, newT.Fields)
	}
}

func diffEnums(r *Report, op, path string, oldVals, newVals []string) {
	oldSet := map[string]bool{}
	for _, v := range oldVals {
		oldSet[v] = true
	}
	newSet := map[string]bool{}
	for _, v := range newVals {
		newSet[v] = true
	}
	// Order matters semantically for some serializations, so report a
	// reordering as breaking with precise detail.
	if len(oldVals) == len(newVals) {
		reordered := false
		for i := range oldVals {
			if oldVals[i] != newVals[i] {
				reordered = true
				break
			}
		}
		if reordered {
			r.add(ChangeBreaking, op, path, fmt.Sprintf(
				"enum value order changed: %s -> %s",
				strings.Join(oldVals, ","), strings.Join(newVals, ",")))
			return
		}
	}
	oldSorted := append([]string(nil), oldVals...)
	newSorted := append([]string(nil), newVals...)
	sort.Strings(oldSorted)
	sort.Strings(newSorted)
	for _, v := range oldSorted {
		if !newSet[v] {
			r.add(ChangeBreaking, op, path+".values."+v,
				"enum value removed; previously produced values may be rejected")
		}
	}
	for _, v := range newSorted {
		if !oldSet[v] {
			r.add(ChangeCompatible, op, path+".values."+v,
				"enum value added (enumeration expansion)")
		}
	}
}

func diffFields(r *Report, op, path string, old, neu []schema.Field) {
	oldMap := map[string]schema.Field{}
	for _, f := range old {
		oldMap[f.Name] = f
	}
	newMap := map[string]schema.Field{}
	for _, f := range neu {
		newMap[f.Name] = f
	}
	names := make([]string, 0, len(old)+len(neu))
	seen := map[string]bool{}
	for _, f := range old {
		names = append(names, f.Name)
		seen[f.Name] = true
	}
	for _, f := range neu {
		if !seen[f.Name] {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		of, inOld := oldMap[n]
		nf, inNew := newMap[n]
		fp := path + ".fields." + n
		switch {
		case inOld && !inNew:
			r.add(ChangeBreaking, op, fp, "field removed; existing callers still produce/read it")
		case !inOld && inNew:
			if nf.Optional {
				r.add(ChangeCompatible, op, fp, "new optional field added")
			} else {
				r.add(ChangeBreaking, op, fp,
					"new field added but it is required; older clients cannot supply it")
			}
		default:
			if of.Optional != nf.Optional {
				r.add(ChangeBreaking, op, fp, fmt.Sprintf(
					"required/optional semantic changed: optional=%v -> optional=%v",
					of.Optional, nf.Optional))
			}
			diffTypes(r, op, fp, of.Type, nf.Type)
		}
	}
}
