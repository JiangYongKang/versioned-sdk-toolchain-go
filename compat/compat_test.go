package compat

import (
	"strings"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
)

func ref(n string) schema.Type { return schema.Type{Kind: schema.TypeRef, RefName: n} }
func str() schema.Type         { return schema.Type{Kind: schema.KindString} }
func integer() schema.Type     { return schema.Type{Kind: schema.KindInteger} }
func enum(vals ...string) schema.Type {
	return schema.Type{Kind: schema.TypeEnum, EnumVals: vals}
}
func obj(fields ...schema.Field) schema.Type {
	return schema.Type{Kind: schema.TypeObject, Fields: fields}
}
func field(name string, t schema.Type, opt ...bool) schema.Field {
	f := schema.Field{Name: name, Type: t}
	if len(opt) > 0 {
		f.Optional = opt[0]
	}
	return f
}
func arr(t schema.Type) schema.Type {
	return schema.Type{Kind: schema.TypeArray, Elem: &t}
}

func named(name string, t schema.Type) schema.NamedType {
	return schema.NamedType{Name: name, Type: t}
}
func op(name string, in, out schema.Type) schema.Operation {
	return schema.Operation{Name: name, Input: in, Output: out}
}

func base() *schema.Interface {
	return &schema.Interface{
		Name:    "billing",
		Version: "v1.0.0",
		Types: []schema.NamedType{
			named("Currency", enum("USD", "EUR")),
			named("Money", obj(
				field("amount_minor", integer()),
				field("currency", ref("Currency")),
			)),
		},
		Operations: []schema.Operation{
			op("create_invoice",
				obj(field("customer_id", str()), field("total", ref("Money"))),
				obj(field("invoice_id", str()), field("status", enum("open", "paid"))),
			),
			op("get_invoice",
				obj(field("invoice_id", str())),
				obj(field("invoice_id", str()), field("total", ref("Money"))),
			),
		},
	}
}

func deepType(t schema.Type) schema.Type {
	cp := t
	cp.EnumVals = append([]string(nil), t.EnumVals...)
	if t.Elem != nil {
		e := deepType(*t.Elem)
		cp.Elem = &e
	}
	if t.Fields != nil {
		cp.Fields = make([]schema.Field, len(t.Fields))
		for i, f := range t.Fields {
			f.Type = deepType(f.Type)
			cp.Fields[i] = f
		}
	}
	return cp
}

func clone(p *schema.Interface) *schema.Interface {
	q := *p
	q.Types = make([]schema.NamedType, len(p.Types))
	for i, nt := range p.Types {
		nt.Type = deepType(nt.Type)
		q.Types[i] = nt
	}
	q.Operations = make([]schema.Operation, len(p.Operations))
	for i, o := range p.Operations {
		o.Input = deepType(o.Input)
		o.Output = deepType(o.Output)
		q.Operations[i] = o
	}
	q.Version = "v2.0.0"
	return &q
}

// printReport logs interface, version pair and each decision basis.
func printReport(t *testing.T, r *Report) {
	t.Helper()
	t.Logf("interface=%s %s -> %s compatible=%v (%d changes)",
		r.Service, r.FromVer, r.ToVer, r.Compatible(), len(r.Changes))
	for _, c := range r.Changes {
		t.Logf("   [%s] %s op=%q :: %s", c.Kind, c.Field, c.Op, c.Detail)
	}
}

func TestCompatibleChanges(t *testing.T) {
	cases := []struct {
		name      string
		apply     func(q *schema.Interface)
		wantField string
	}{
		{
			name: "add optional field",
			apply: func(q *schema.Interface) {
				q.Types[1].Type.Fields = append(q.Types[1].Type.Fields,
					field("memo", str(), true))
			},
			wantField: "types.Money.fields.memo",
		},
		{
			name: "enum expansion",
			apply: func(q *schema.Interface) {
				q.Types[0].Type.EnumVals = append(q.Types[0].Type.EnumVals, "GBP")
			},
			wantField: "types.Currency.values.GBP",
		},
		{
			name: "add new operation",
			apply: func(q *schema.Interface) {
				q.Operations = append(q.Operations, op("list_invoices",
					obj(field("customer_id", str())),
					obj(field("ids", arr(str())))))
			},
			wantField: "operations.list_invoices",
		},
		{
			name: "add new named type",
			apply: func(q *schema.Interface) {
				q.Types = append(q.Types, named("Address", obj(
					field("city", str()), field("zip", str(), true))))
			},
			wantField: "types.Address",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := clone(base())
			tc.apply(next)
			r, err := Compare(base(), next)
			if err != nil {
				t.Fatal(err)
			}
			printReport(t, r)
			if !r.Compatible() {
				t.Fatalf("expected compatible, breaking=%v", r.BreakingChanges())
			}
			found := false
			for _, c := range r.Changes {
				if c.Field == tc.wantField && c.Kind == ChangeCompatible {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a compatible change at %s", tc.wantField)
			}
		})
	}
}

func TestBreakingChanges(t *testing.T) {
	cases := []struct {
		name      string
		apply     func(q *schema.Interface)
		wantField string
		basis     string
	}{
		{
			name: "remove field",
			apply: func(q *schema.Interface) {
				q.Types[1].Type.Fields = q.Types[1].Type.Fields[:1]
			},
			wantField: "types.Money.fields.currency",
			basis:     "field removed",
		},
		{
			name: "change field type",
			apply: func(q *schema.Interface) {
				q.Types[1].Type.Fields[0].Type = str()
			},
			wantField: "types.Money.fields.amount_minor",
			basis:     "type kind changed",
		},
		{
			name: "remove enum value",
			apply: func(q *schema.Interface) {
				q.Types[0].Type.EnumVals = []string{"USD"}
			},
			wantField: "types.Currency.values.EUR",
			basis:     "enum value removed",
		},
		{
			name: "reorder enum values",
			apply: func(q *schema.Interface) {
				q.Types[0].Type.EnumVals = []string{"EUR", "USD"}
			},
			wantField: "types.Currency",
			basis:     "order changed",
		},
		{
			name: "remove operation",
			apply: func(q *schema.Interface) {
				q.Operations = q.Operations[:1]
			},
			wantField: "operations.get_invoice",
			basis:     "operation removed",
		},
		{
			name: "optional to required",
			apply: func(q *schema.Interface) {
				q.Operations[0].Input.Fields = append(
					q.Operations[0].Input.Fields, field("promo", str(), true))
				// baseline must also have it: build fresh prev with promo optional.
			},
			wantField: "__special__",
		},
		{
			name: "new required field is breaking",
			apply: func(q *schema.Interface) {
				q.Types[1].Type.Fields = append(q.Types[1].Type.Fields,
					field("mandatory_memo", str()))
			},
			wantField: "types.Money.fields.mandatory_memo",
			basis:     "required",
		},
		{
			name: "ref target changed",
			apply: func(q *schema.Interface) {
				q.Types = append(q.Types, named("OtherMoney", obj(field("x", str()))))
				q.Types[1].Type.Fields = []schema.Field{
					field("amount_minor", integer()),
					field("currency", ref("Currency")),
				}
				// get_invoice output fields: invoice_id(0), total(1).
				q.Operations[1].Output.Fields[1].Type = ref("OtherMoney")
			},
			wantField: "operations.get_invoice.output.fields.total",
			basis:     "reference target changed",
		},
		{
			name: "array element type changed",
			apply: func(q *schema.Interface) {
				q.Operations = append(q.Operations, op("list_ids",
					obj(field("ids", arr(str()))),
					obj(field("ok", schema.Type{Kind: schema.KindBoolean}))))
			},
			wantField: "__arr_setup__",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			switch tc.name {
			case "optional to required":
				p = clone(base())
				p.Version = "v1.0.0"
				p.Types[1].Type.Fields = append(p.Types[1].Type.Fields,
					field("promo", str(), true))
				next := clone(p)
				for i := range next.Types[1].Type.Fields {
					if next.Types[1].Type.Fields[i].Name == "promo" {
						next.Types[1].Type.Fields[i].Optional = false
					}
				}
				r, err := Compare(p, next)
				if err != nil {
					t.Fatal(err)
				}
				printReport(t, r)
				if r.Compatible() {
					t.Fatal("optional->required must be breaking")
				}
				found := false
				for _, c := range r.BreakingChanges() {
					if c.Field == "types.Money.fields.promo" &&
						strings.Contains(c.Detail, "required/optional") {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing optional/required breaking change")
				}
				return
			case "array element type changed":
				// prev list_ids: []string; next: []integer -> breaking.
				p = clone(base())
				p.Version = "v1.0.0"
				p.Operations = append(p.Operations, op("list_ids",
					obj(field("ids", arr(str()))),
					obj(field("ok", schema.Type{Kind: schema.KindBoolean}))))
				next := clone(p)
				next.Operations[len(next.Operations)-1].Input.Fields[0].Type =
					arr(integer())
				r, err := Compare(p, next)
				if err != nil {
					t.Fatal(err)
				}
				printReport(t, r)
				if r.Compatible() {
					t.Fatal("array element change must be breaking")
				}
				return
			}

			next := clone(p)
			tc.apply(next)
			r, err := Compare(p, next)
			if err != nil {
				t.Fatal(err)
			}
			printReport(t, r)
			if r.Compatible() {
				t.Fatalf("expected breaking changes, report compatible")
			}
			found := false
			for _, c := range r.BreakingChanges() {
				if c.Field == tc.wantField && strings.Contains(c.Detail, tc.basis) {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected breaking change at %q mentioning %q; got %+v",
					tc.wantField, tc.basis, r.BreakingChanges())
			}
		})
	}
}

func TestNoChangesIsCompatible(t *testing.T) {
	next := clone(base())
	r, err := Compare(base(), next)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Compatible() || len(r.Changes) != 0 {
		printReport(t, r)
		t.Fatalf("identical interfaces must yield zero changes, got %+v", r.Changes)
	}
	t.Logf("interface=%s %s->%s: no changes, basis=structural equality",
		r.Service, r.FromVer, r.ToVer)
}

func TestCompareRejectsDifferentServices(t *testing.T) {
	a := base()
	b := clone(base())
	b.Name = "other"
	if _, err := Compare(a, b); err == nil {
		t.Fatal("comparing different services must error")
	}
}
