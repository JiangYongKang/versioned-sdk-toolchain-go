package codegen

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

func sampleBundle(t *testing.T, ver string) *schema.Bundle {
	t.Helper()
	v, err := schema.ParseVersion(ver)
	if err != nil {
		t.Fatal(err)
	}
	return &schema.Bundle{Documents: []*schema.Document{{
		Service: "petstore", Version: v, Source: ver + ".json",
		Enums: []schema.Enum{{Name: "Kind", Values: []string{"a", "b"}}},
		Structs: []schema.Struct{{Name: "Pet", Fields: []schema.Field{
			{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}},
			{Name: "kind", Type: schema.TypeRef{Name: "Kind"}},
		}}},
		Interfaces: []schema.Interface{{Name: "GetPet", Method: "M",
			Input:  []schema.Parameter{{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}}},
			Output: &schema.TypeRef{Name: "Pet"},
		}},
	}}}
}

func readArtifacts(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRepeatedGenerationIsStable(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	b := sampleBundle(t, "1.0.0")
	r1, err := Generate(b, Options{OutputDir: dir1})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Generate(b, Options{OutputDir: dir2})
	if err != nil {
		t.Fatal(err)
	}
	a1 := readArtifacts(t, dir1)
	a2 := readArtifacts(t, dir2)
	if len(a1) != len(a2) {
		t.Fatalf("artifact counts differ: %d vs %d", len(a1), len(a2))
	}
	for name, content := range a1 {
		if a2[name] != content {
			t.Fatalf("artifact %s not stable across repeated generations", name)
		}
		if content != "" && content[len(content)-1] != '\n' {
			t.Fatalf("artifact %s does not end with newline", name)
		}
	}
	if string(r1.Metadata) != string(r2.Metadata) {
		t.Fatal("metadata bytes differ across generations")
	}
	t.Logf("service=petstore version=1.0.0 artifacts=%v repeated generation byte-identical", r1.Artifacts)
}

func TestConcurrentGenerationNoCorruption(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sdk")
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = Generate(sampleBundle(t, "1.0.0"), Options{OutputDir: dir})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}
	// Post-condition: the directory must be one complete, consistent SDK
	// (all 5 files, no staging leftovers).
	arts := readArtifacts(t, dir)
	want := map[string]bool{
		"doc.go": false, "errors.go": false, "types.go": false,
		"client.go": false, "metadata.json": false, "smoke_test.go": false,
	}
	for name := range arts {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected/leftover artifact after concurrent generation: %q", name)
		}
		want[name] = true
	}
	for name, ok := range want {
		if !ok {
			t.Fatalf("missing artifact after concurrent generation: %s", name)
		}
	}
	t.Logf("interface=GetPet version=1.0.0 concurrency=%d verdict=stable no half-written output", n)
}

func TestGenerateRejectsInvalidBundle(t *testing.T) {
	bad := &schema.Bundle{Documents: []*schema.Document{{
		Service: "s", Version: mustV(t, "1.0.0"),
		Structs: []schema.Struct{{Name: "P", Fields: []schema.Field{
			{Name: "f", Type: schema.TypeRef{Name: "Missing"}},
		}}},
	}}}
	_, err := Generate(bad, Options{OutputDir: t.TempDir()})
	if apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
}

func TestGenerateFailureCategory(t *testing.T) {
	// An unwritable output path (file where a dir is needed) yields
	// generation_failed, not a raw os error.
	parent := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Generate(sampleBundle(t, "1.0.0"),
		Options{OutputDir: filepath.Join(parent, "sub")})
	if apierrors.Code(err) != apierrors.CatGenerationFailed {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
}

func mustV(t *testing.T, s string) schema.Version {
	v, err := schema.ParseVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
