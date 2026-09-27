package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeSchema marshals v as JSON to path, creating parent directories.
func writeSchema(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

type fieldSpecT struct {
	name     string
	kind     string
	optional bool
}

func fieldSpec(name, kind string, optional bool) fieldSpecT {
	return fieldSpecT{name: name, kind: kind, optional: optional}
}

func objectType(fields ...fieldSpecT) map[string]any {
	fs := make([]any, 0, len(fields))
	for _, f := range fields {
		entry := map[string]any{
			"name":     f.name,
			"optional": f.optional,
			"type":     map[string]any{"kind": f.kind},
		}
		fs = append(fs, entry)
	}
	return map[string]any{"kind": "object", "fields": fs}
}
