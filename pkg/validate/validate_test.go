package validate

import (
	"errors"
	"strings"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

func mustBundle(t *testing.T, docs ...*schema.Document) *schema.Bundle {
	t.Helper()
	b := &schema.Bundle{Documents: docs}
	return b
}

func ref(name string) schema.TypeRef         { return schema.TypeRef{Name: name} }
func prim(p schema.Primitive) schema.TypeRef { return schema.TypeRef{Primitive: p} }

func TestMissingReference(t *testing.T) {
	b := mustBundle(t, &schema.Document{
		Service: "s", Version: v(t, "1.0.0"), Source: "a.json",
		Structs: []schema.Struct{{Name: "Pet", Fields: []schema.Field{
			{Name: "owner", Type: ref("Ghost")},
		}}},
		Interfaces: []schema.Interface{{Name: "GetPet", Method: "M", Output: ptr(ref("Missing"))}},
	})
	err := Validate(b)
	if err == nil {
		t.Fatal("expected error for missing references")
	}
	if apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("category = %s", apierrors.Code(err))
	}
	var me *apierrors.MultiError
	if !errors.As(err, &me) || me.Len() < 2 {
		t.Fatalf("expected >=2 issues, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"Pet.owner", "missing type \"Ghost\"", "GetPet", "missing type \"Missing\""} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	t.Logf("interface=GetPet version=1.0.0 -> missing refs rejected with field-level reasons:\n%s", msg)
}

func TestTypeConflict(t *testing.T) {
	b := mustBundle(t,
		&schema.Document{Service: "s", Version: v(t, "1.0.0"), Source: "a.json",
			Structs: []schema.Struct{{Name: "Pet", Fields: []schema.Field{{Name: "id", Type: prim(schema.PrimLong)}}}}},
		&schema.Document{Service: "s", Version: v(t, "1.0.0"), Source: "b.json",
			Structs: []schema.Struct{{Name: "Pet", Fields: []schema.Field{{Name: "id", Type: prim(schema.PrimString)}}}}},
	)
	err := Validate(b)
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("expected conflict error, got %v", err)
	}
	t.Logf("service=s version=1.0.0 -> type conflict located across a.json/b.json:\n%s", err)
}

func TestCycle(t *testing.T) {
	b := mustBundle(t, &schema.Document{Service: "s", Version: v(t, "1.0.0"), Source: "a.json",
		Structs: []schema.Struct{
			{Name: "A", Fields: []schema.Field{{Name: "b", Type: ref("B")}}},
			{Name: "B", Fields: []schema.Field{{Name: "a", Type: ref("A")}}},
		},
	})
	err := Validate(b)
	if err == nil || !strings.Contains(err.Error(), "circular reference") {
		t.Fatalf("expected cycle error, got %v", err)
	}
	if !strings.Contains(err.Error(), "A -> B -> A") {
		t.Fatalf("cycle path not rendered: %v", err)
	}
	t.Logf("interface=<structs> version=1.0.0 -> cycle rejected, basis:\n%s", err)
}

func TestDuplicateNames(t *testing.T) {
	b := mustBundle(t, &schema.Document{Service: "s", Version: v(t, "1.0.0"), Source: "a.json",
		Structs: []schema.Struct{{Name: "P", Fields: []schema.Field{
			{Name: "x", Type: prim(schema.PrimString)},
			{Name: "x", Type: prim(schema.PrimInt)},
		}}},
		Enums: []schema.Enum{{Name: "E", Values: []string{"a", "a"}}},
	})
	err := Validate(b)
	if err == nil {
		t.Fatal("expected duplicate name errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "duplicate struct field") || !strings.Contains(msg, "duplicate enum value") {
		t.Fatalf("missing duplicate findings: %s", msg)
	}
}

func TestValidBundle(t *testing.T) {
	b := mustBundle(t, &schema.Document{Service: "s", Version: v(t, "1.0.0"), Source: "a.json",
		Enums: []schema.Enum{{Name: "Kind", Values: []string{"a", "b"}}},
		Structs: []schema.Struct{{Name: "P", Fields: []schema.Field{
			{Name: "k", Type: ref("Kind")},
			{Name: "ids", Type: schema.TypeRef{Array: ptr(prim(schema.PrimLong))}},
		}}},
		Interfaces: []schema.Interface{{Name: "Get", Method: "M", Output: ptr(ref("P"))}},
	})
	if err := Validate(b); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
}

func v(t *testing.T, s string) schema.Version {
	t.Helper()
	x, err := schema.ParseVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func ptr(t schema.TypeRef) *schema.TypeRef { return &t }

func TestCrossDocumentFragmentMergeAndConflict(t *testing.T) {
	// Two fragments of the same service/version merge cleanly.
	merged := mustBundle(t,
		&schema.Document{Service: "shop", Version: v(t, "1.0.0"), Source: "common.json",
			Structs: []schema.Struct{{Name: "Money", Fields: []schema.Field{
				{Name: "amount", Type: prim(schema.PrimDouble)},
			}}}},
		&schema.Document{Service: "shop", Version: v(t, "1.0.0"), Source: "orders.json",
			Structs: []schema.Struct{{Name: "Order", Fields: []schema.Field{
				{Name: "total", Type: ref("Money")},
			}}}},
	)
	if err := Validate(merged); err != nil {
		t.Fatalf("fragments should merge: %v", err)
	}

	// But a third fragment redefining Money differently is a conflict.
	conflict := mustBundle(t,
		merged.Documents[0], merged.Documents[1],
		&schema.Document{Service: "shop", Version: v(t, "1.0.0"), Source: "billing.json",
			Structs: []schema.Struct{{Name: "Money", Fields: []schema.Field{
				{Name: "amount", Type: prim(schema.PrimString)},
			}}}},
	)
	err := Validate(conflict)
	if err == nil || !contains(err.Error(), "conflicting") {
		t.Fatalf("expected cross-document conflict, got %v", err)
	}
	t.Logf("service=shop version=1.0.0 cross-document conflict rejected:\n%s", err)
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
