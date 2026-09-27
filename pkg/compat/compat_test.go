package compat

import (
	"strings"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

func ptr(t schema.TypeRef) *schema.TypeRef { return &t }

func doc(t *testing.T, ver string,
	structs []schema.Struct, enums []schema.Enum, ifaces []schema.Interface) *schema.Document {
	v, err := schema.ParseVersion(ver)
	if err != nil {
		t.Fatal(err)
	}
	return &schema.Document{Service: "s", Version: v, Source: ver + ".json",
		Structs: structs, Enums: enums, Interfaces: ifaces}
}

func TestCompatibleChanges(t *testing.T) {
	old := &schema.Bundle{Documents: []*schema.Document{doc(t, "1.0.0",
		[]schema.Struct{{Name: "Pet", Fields: []schema.Field{
			{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}},
			{Name: "n", Type: schema.TypeRef{Primitive: schema.PrimInt}},
		}}},
		[]schema.Enum{{Name: "K", Values: []string{"a"}}},
		[]schema.Interface{{Name: "Get", Method: "M",
			Input:  []schema.Parameter{{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}}},
			Output: ptr(schema.TypeRef{Name: "Pet"}),
		}},
	)}}
	neu := &schema.Bundle{Documents: []*schema.Document{doc(t, "1.1.0",
		[]schema.Struct{{Name: "Pet", Fields: []schema.Field{
			{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}},
			{Name: "n", Type: schema.TypeRef{Primitive: schema.PrimLong}}, // widen int->long
			{Name: "tag", Type: schema.TypeRef{Primitive: schema.PrimString}, Optional: true},
		}}},
		[]schema.Enum{{Name: "K", Values: []string{"a", "b"}}}, // enum expansion
		[]schema.Interface{{Name: "Get", Method: "M",
			Input: []schema.Parameter{
				{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}},
				{Name: "expand", Type: schema.TypeRef{Primitive: schema.PrimBool}, Optional: true},
			},
			Output: ptr(schema.TypeRef{Name: "Pet"}),
		}},
	)}}
	r, err := Compare(old, neu)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Compatible() {
		t.Fatalf("expected compatible, breaking changes:\n%v", r.Breaking())
	}
	t.Logf("interface=Get version=1.0.0->1.1.0 verdict=compatible basis:")
	for _, c := range r.Changes {
		t.Logf("  [%s] iface=%s field=%s reason=%s", c.Kind, c.Interface, c.Field, c.Reason)
	}
}

func TestBreakingChanges(t *testing.T) {
	old := &schema.Bundle{Documents: []*schema.Document{doc(t, "1.0.0",
		[]schema.Struct{{Name: "Pet", Fields: []schema.Field{
			{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}},
			{Name: "name", Type: schema.TypeRef{Primitive: schema.PrimString}},
		}}},
		[]schema.Enum{{Name: "K", Values: []string{"a", "b"}}},
		[]schema.Interface{{Name: "Get", Method: "M",
			Input:  []schema.Parameter{{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}}},
			Output: ptr(schema.TypeRef{Name: "Pet"}),
		}, {Name: "Old", Method: "M2"}},
	)}}
	neu := &schema.Bundle{Documents: []*schema.Document{doc(t, "2.0.0",
		[]schema.Struct{{Name: "Pet", Fields: []schema.Field{
			{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimString}}, // type change
			// name removed
		}}},
		[]schema.Enum{{Name: "K", Values: []string{"a"}}}, // enum value removed
		[]schema.Interface{{Name: "Get", Method: "M",
			Input: []schema.Parameter{{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimString}}},
		}},
	)}}
	r, err := Compare(old, neu)
	if err != nil {
		t.Fatal(err)
	}
	if r.Compatible() {
		t.Fatal("expected breaking report")
	}
	msg := ""
	hasField, hasType, hasEnum, hasIfaceRemoved, hasOutputRemoved := false, false, false, false, false
	for _, c := range r.Breaking() {
		msg += c.Field + ":" + c.Reason + "\n"
		switch {
		case c.Field == "Pet.name" && strings.Contains(c.Reason, "field removed"):
			hasField = true
		case c.Field == "Pet.id" && strings.Contains(c.Reason, "primitive changed"):
			hasType = true
		case strings.HasPrefix(c.Field, "K.") && strings.Contains(c.Reason, "enum value removed"):
			hasEnum = true
		case c.Interface == "Old" && strings.Contains(c.Reason, "interface removed"):
			hasIfaceRemoved = true
		case c.Interface == "Get" && strings.Contains(c.Reason, "output removed"):
			hasOutputRemoved = true
		}
	}
	for name, got := range map[string]bool{
		"field removed": hasField, "type changed": hasType,
		"enum removed": hasEnum, "interface removed": hasIfaceRemoved,
		"output removed": hasOutputRemoved,
	} {
		if !got {
			t.Errorf("missing breaking finding for %s\n%s", name, msg)
		}
	}
	t.Logf("interface=Get,Old version=1.0.0->2.0.0 verdict=breaking affected scope:\n%s", msg)
}

func TestIllegalVersionProgression(t *testing.T) {
	old := &schema.Bundle{Documents: []*schema.Document{doc(t, "1.2.0", nil, nil, nil)}}
	neu := &schema.Bundle{Documents: []*schema.Document{doc(t, "1.1.0", nil, nil, nil)}}
	_, err := Compare(old, neu)
	if apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("expected invalid_input, got %v", err)
	}
	t.Logf("version=1.2.0->1.1.0 rejected as invalid_input: %v", err)
}
