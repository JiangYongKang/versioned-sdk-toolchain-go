package load

import (
	"testing"
	"testing/fstest"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
)

func TestLoadOK(t *testing.T) {
	fsys := fstest.MapFS{
		"a.json": &fstest.MapFile{Data: []byte(`{
			"service": "s", "version": "1.0.0",
			"enums": [{"name": "K", "values": ["x"]}],
			"structs": [{"name": "P", "fields": [
				{"name": "id", "type": {"primitive": "long"}}
			]}],
			"interfaces": [{"name": "Get", "method": "M",
				"input": [{"name": "id", "type": {"primitive": "long"}}],
				"output": {"ref": "P"}}]
		}`)},
	}
	b, err := NewFS(fsys).LoadFiles("a.json")
	if err != nil {
		t.Fatal(err)
	}
	if b.Documents[0].Service != "s" || b.Documents[0].Version.String() != "1.0.0" {
		t.Fatalf("decoded doc wrong: %+v", b.Documents[0])
	}
	if b.Documents[0].Structs[0].Fields[0].Type.Primitive != "long" {
		t.Fatal("primitive not decoded")
	}
}

func TestLoadMalformed(t *testing.T) {
	cases := map[string]string{
		"bad json":      `{"service":`,
		"unknown field": `{"service":"s","version":"1.0.0","bogus":1}`,
		"bad version":   `{"service":"s","version":"1.0"}`,
		"no service":    `{"version":"1.0.0"}`,
		"double ref": `{"service":"s","version":"1.0.0",
			"structs":[{"name":"P","fields":[{"name":"f","type":{"ref":"X","primitive":"string"}}]}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{"bad.json": &fstest.MapFile{Data: []byte(content)}}
			_, err := NewFS(fsys).LoadFiles("bad.json")
			if err == nil {
				t.Fatalf("%s: expected error", name)
			}
			if apierrors.Code(err) != apierrors.CatInvalidInput {
				t.Fatalf("%s: category=%s", name, apierrors.Code(err))
			}
			t.Logf("case=%s file=bad.json verdict=invalid_input reason=%v", name, err)
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := New().LoadFiles("/nonexistent/does-not-exist.json")
	if apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
}
