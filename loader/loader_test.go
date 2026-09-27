package loader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadOne(t *testing.T, content string) error {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "svc/v1.json", content)
	l := New(Options{})
	_, err := l.LoadDir(context.Background(), dir)
	return err
}

const validSvc = `{
  "name": "svc", "version": "v1.0.0",
  "operations": [
    {"name":"ping","input":{"kind":"object","fields":[
      {"name":"x","type":{"kind":"string"}}]},
     "output":{"kind":"object","fields":[
      {"name":"y","type":{"kind":"string"}}]}}
  ]
}`

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "svc/v1.json", validSvc)
	l := New(Options{})
	doc, err := l.LoadDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Bundles) != 1 || doc.Bundles[0].Interface.Name != "svc" {
		t.Fatalf("unexpected document: %+v", doc)
	}
}

// TestMalformedInputs asserts each malformed input is a KindInput error
// and that the diagnostic identifies the concrete field and reason.
func TestMalformedInputs(t *testing.T) {
	cases := []struct {
		name    string
		content string
		kind    sdkerrors.Kind
		field   string // fragment expected in the field path
		reason  string // fragment expected in the reason
	}{
		{
			name:    "invalid json",
			content: `{"name": "svc", broken`,
			kind:    sdkerrors.KindInput,
			reason:  "invalid JSON",
		},
		{
			name: "illegal version",
			content: `{"name":"svc","version":"not-a-version",
			  "operations":[{"name":"p","input":{"kind":"object","fields":[]},
			  "output":{"kind":"object","fields":[]}}]}`,
			kind:   sdkerrors.KindInput,
			reason: "illegal version",
		},
		{
			name: "missing name",
			content: `{"version":"v1","operations":[{"name":"p",
			  "input":{"kind":"object","fields":[]},
			  "output":{"kind":"object","fields":[]}}]}`,
			kind:   sdkerrors.KindInput,
			field:  "name",
			reason: "missing interface name",
		},
		{
			name: "missing reference",
			content: `{"name":"svc","version":"v1","operations":[
			  {"name":"p","input":{"kind":"object","fields":[
			    {"name":"a","type":{"kind":"ref","ref":"Ghost"}}]},
			    "output":{"kind":"object","fields":[
			    {"name":"b","type":{"kind":"string"}}]}}]}`,
			kind:   sdkerrors.KindInput,
			field:  "Ghost",
			reason: "missing reference",
		},
		{
			name: "duplicate named type",
			content: `{"name":"svc","version":"v1","types":[
			  {"name":"T","type":{"kind":"string"}},
			  {"name":"T","type":{"kind":"integer"}}],
			  "operations":[{"name":"p","input":{"kind":"string"},"output":{"kind":"string"}}]}`,
			kind:   sdkerrors.KindInput,
			field:  "types.T",
			reason: "duplicate named type",
		},
		{
			name: "duplicate operation",
			content: `{"name":"svc","version":"v1","operations":[
			  {"name":"p","input":{"kind":"string"},"output":{"kind":"string"}},
			  {"name":"p","input":{"kind":"string"},"output":{"kind":"string"}}]}`,
			kind:   sdkerrors.KindInput,
			field:  "operations.p",
			reason: "duplicate operation",
		},
		{
			name: "duplicate enum value",
			content: `{"name":"svc","version":"v1","operations":[
			  {"name":"p","input":{"kind":"enum","values":["A","A"]},"output":{"kind":"string"}}]}`,
			kind:   sdkerrors.KindInput,
			reason: "duplicate value",
		},
		{
			name: "enum without values",
			content: `{"name":"svc","version":"v1","operations":[
			  {"name":"p","input":{"kind":"enum"},"output":{"kind":"string"}}]}`,
			kind:   sdkerrors.KindInput,
			reason: "declares no values",
		},
		{
			name: "array without element",
			content: `{"name":"svc","version":"v1","operations":[
			  {"name":"p","input":{"kind":"array"},"output":{"kind":"string"}}]}`,
			kind:   sdkerrors.KindInput,
			field:  "elem",
			reason: "missing element type",
		},
		{
			name: "object duplicate field",
			content: `{"name":"svc","version":"v1","operations":[
			  {"name":"p","input":{"kind":"object","fields":[
			    {"name":"a","type":{"kind":"string"}},
			    {"name":"a","type":{"kind":"integer"}}]},
			    "output":{"kind":"string"}}]}`,
			kind:   sdkerrors.KindInput,
			field:  "fields.a",
			reason: "duplicate field name",
		},
		{
			name: "unknown type kind",
			content: `{"name":"svc","version":"v1","operations":[
			  {"name":"p","input":{"kind":"widget"},"output":{"kind":"string"}}]}`,
			kind:   sdkerrors.KindInput,
			reason: "unknown type kind",
		},
		{
			name:    "no operations",
			content: `{"name":"svc","version":"v1","operations":[]}`,
			kind:    sdkerrors.KindInput,
			reason:  "no operations",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := loadOne(t, tc.content)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.name)
			}
			se := sdkerrors.AsError(err)
			if se == nil {
				t.Fatalf("error is not classified: %v", err)
			}
			if se.Kind != tc.kind {
				t.Fatalf("[%s] kind=%s want %s | full=%v", tc.name, se.Kind, tc.kind, err)
			}
			if tc.field != "" {
				inField := strings.Contains(se.Field, tc.field)
				inCause := strings.Contains(err.Error(), tc.field)
				if !inField && !inCause {
					t.Fatalf("[%s] neither field=%q nor cause mentions %q | full=%v",
						tc.name, se.Field, tc.field, err)
				}
			}
			if tc.reason != "" && !strings.Contains(se.Reason, tc.reason) {
				t.Fatalf("[%s] reason=%q does not mention %q | full=%v",
					tc.name, se.Reason, tc.reason, err)
			}
			t.Logf("[%s] service=%s version=%s field=%s => %v",
				tc.name, se.Service, se.Version, se.Field, err)
		})
	}
}

func TestCycleDetected(t *testing.T) {
	// A -> B -> A through object fields.
	content := `{
	  "name":"svc","version":"v1",
	  "types":[
	    {"name":"A","type":{"kind":"object","fields":[
	      {"name":"b","type":{"kind":"ref","ref":"B"}}]}},
	    {"name":"B","type":{"kind":"object","fields":[
	      {"name":"a","type":{"kind":"ref","ref":"A"}}]}}
	  ],
	  "operations":[
	    {"name":"p","input":{"kind":"ref","ref":"A"},"output":{"kind":"string"}}
	  ]}`
	err := loadOne(t, content)
	se := sdkerrors.AsError(err)
	if se == nil || se.Kind != sdkerrors.KindInput {
		t.Fatalf("expected KindInput for cycle, got %v", err)
	}
	if !strings.Contains(err.Error(), "circular reference") ||
		!strings.Contains(se.Field, "A") || !strings.Contains(se.Field, "B") {
		t.Fatalf("cycle diagnostic lacks chain detail: %v", err)
	}
	t.Logf("cycle rejected at service=%s version=%s chain=%s",
		se.Service, se.Version, se.Field)
}

func TestDuplicateVersionAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "svc/v1.json", validSvc)
	writeFile(t, dir, "svc/svc@v1.0.0.json", validSvc)
	_, err := New(Options{}).LoadDir(context.Background(), dir)
	se := sdkerrors.AsError(err)
	if se == nil || se.Kind != sdkerrors.KindInput {
		t.Fatalf("expected KindInput for duplicate version, got %v", err)
	}
	t.Logf("duplicate version rejected: %v", err)
}

func TestEmptyDirectory(t *testing.T) {
	_, err := New(Options{}).LoadDir(context.Background(), t.TempDir())
	if !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("expected KindInput for empty dir, got %v", err)
	}
}

func TestUnknownCodecRejectedWhenRegistryProvided(t *testing.T) {
	content := `{"name":"svc","version":"v1","codec":"msgpack",
	  "operations":[{"name":"p","input":{"kind":"string"},"output":{"kind":"string"}}]}`
	dir := t.TempDir()
	writeFile(t, dir, "svc/v1.json", content)
	l := New(Options{KnownCodecs: map[string]bool{"json": true}})
	_, err := l.LoadDir(context.Background(), dir)
	se := sdkerrors.AsError(err)
	if se == nil || se.Kind != sdkerrors.KindInput {
		t.Fatalf("unknown codec must be KindInput, got %v", err)
	}
	if se.Field != "codec" {
		t.Fatalf("error must point at field codec, got %q", se.Field)
	}
	t.Logf("unknown codec rejected interface=%s version=%s: %v",
		se.Service, se.Version, err)
}

func TestUnknownCodecAllowedWhenNoRegistry(t *testing.T) {
	// Backwards-compatible: without a registry wired in, codec names are
	// only syntax-checked.
	content := `{"name":"svc","version":"v1","codec":"msgpack",
	  "operations":[{"name":"p","input":{"kind":"string"},"output":{"kind":"string"}}]}`
	dir := t.TempDir()
	writeFile(t, dir, "svc/v1.json", content)
	if _, err := New(Options{}).LoadDir(context.Background(), dir); err != nil {
		t.Fatalf("expected load without registry to succeed, got %v", err)
	}
}
