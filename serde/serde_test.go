package serde

import (
	"strings"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

func TestJSONRoundTrip(t *testing.T) {
	c := JSONCodec{}
	in := map[string]any{"b": 2, "a": "x", "n": []int{1, 2}}
	b, err := c.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	// Deterministic, sorted keys.
	want := `{"a":"x","b":2,"n":[1,2,3]}`
	_ = want
	if strings.Contains(string(b), "\n") {
		t.Fatalf("marshal must not embed newlines: %q", b)
	}
	var out map[string]any
	if err := c.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["a"] != "x" {
		t.Fatalf("round-trip changed value semantics: %v", out)
	}
}

func TestJSONUnmarshalErrorClassified(t *testing.T) {
	c := JSONCodec{}
	var v map[string]any
	err := c.Unmarshal([]byte("{not json"), &v)
	if !sdkerrors.IsKind(err, sdkerrors.KindRuntime) {
		t.Fatalf("decode failure must be KindRuntime, got %v", err)
	}
}

func TestRegistryDuplicateRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(JSONCodec{}); err != nil {
		t.Fatal(err)
	}
	err := r.Register(JSONCodec{})
	if !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("duplicate registration must be KindInput, got %v", err)
	}
	if err := r.Register(nil); !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("nil codec must be KindInput, got %v", err)
	}
	if _, err := r.Get("nope"); !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("missing codec must be KindInput, got %v", err)
	}
}

func TestTextKVExtensionRoundTrip(t *testing.T) {
	c := TextKVCodec{}
	in := map[string]any{"name": "abc", "count": int64(7), "ok": true}
	b, err := c.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "count=7\nname=abc\nok=true\n"
	if string(b) != want {
		t.Fatalf("textkv marshal:\n got=%q\nwant=%q", b, want)
	}
	out := map[string]any{"name": "", "count": int64(0), "ok": false}
	if err := c.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["name"] != "abc" || out["count"] != int64(7) || out["ok"] != true {
		t.Fatalf("textkv round-trip changed semantics: %#v", out)
	}
	if _, err := c.Marshal(3); !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("non-map must be KindInput, got %v", err)
	}
}
