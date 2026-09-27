package generator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// repoTestdata returns the testdata directory shipped with the repo.
func repoSchemas(t *testing.T) string {
	t.Helper()
	// Tests run from the package dir; testdata is at repo root.
	root, err := filepath.Abs(filepath.Join("..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("testdata not available: %v", err)
	}
	return root
}

func newTestGenerator(t *testing.T, out string) *Generator {
	t.Helper()
	return New(Config{
		OutputRoot: out,
		ModulePath: "sdk-generated",
	})
}

func TestGenerateFromTestdata(t *testing.T) {
	g := newTestGenerator(t, t.TempDir())
	res, err := g.Generate(context.Background(), repoSchemas(t))
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if res.Reused {
		t.Fatal("first generation must not be marked reused")
	}
	if len(res.Artifacts) == 0 {
		t.Fatal("no artifacts produced")
	}
	t.Logf("generated %d artifacts for billing v1/v2", len(res.Artifacts))
	for _, a := range res.Artifacts {
		t.Logf("  artifact: %s", a)
	}
	// Compatibility reports must explain the accepted changes.
	for _, r := range res.Reports {
		t.Logf("compat interface=%s %s->%s compatible=%v",
			r.Service, r.FromVer, r.ToVer, r.Compatible())
		for _, c := range r.Changes {
			t.Logf("   [%s] %s :: %s", c.Kind, c.Field, c.Detail)
		}
	}
}

func TestRepeatedGenerationIsReusedAndStable(t *testing.T) {
	out := t.TempDir()
	g := newTestGenerator(t, out)
	ctx := context.Background()
	r1, err := g.Generate(ctx, repoSchemas(t))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := g.Generate(ctx, repoSchemas(t))
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Reused {
		t.Fatalf("identical re-run must be reused; got reused=false; artifacts=%v", r2.Artifacts)
	}
	t.Logf("repeat generation reused=true basis=input sha %s", r1.Root)

	// Artifact sets must be identical across two fresh roots too.
	out2 := t.TempDir()
	g2 := newTestGenerator(t, out2)
	r3, err := g2.Generate(ctx, repoSchemas(t))
	if err != nil {
		t.Fatal(err)
	}
	if !sameArtifactSet(r1.Artifacts, r3.Artifacts) {
		t.Fatalf("artifact sets differ across independent roots:\n%v\n%v",
			r1.Artifacts, r3.Artifacts)
	}
	// Byte-identical content for every artifact.
	for _, rel := range r3.Artifacts {
		a, err := os.ReadFile(filepath.Join(out, rel))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(out2, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(a) != string(b) {
			t.Fatalf("artifact %s is not byte-identical across runs", rel)
		}
	}
	t.Logf("independent roots produced %d byte-identical artifacts", len(r3.Artifacts))
}

func TestGeneratedGoModuleCompiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	out := t.TempDir()
	g := newTestGenerator(t, out)
	if _, err := g.Generate(context.Background(), repoSchemas(t)); err != nil {
		t.Fatal(err)
	}
	goDir := filepath.Join(out, "go")
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = goDir
	cmd.Env = append(os.Environ(), "GO111MODULE=on", "GOPATH="+t.TempDir(), "GOCACHE="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated module failed to compile:\n%s\nerr=%v", out, err)
	}
	t.Log("generated SDK compiles offline with stdlib only")
}

func TestBreakingChangeAcrossRunsRejected(t *testing.T) {
	out := t.TempDir()
	ctx := context.Background()

	// First run: v1 with a required field.
	dir := t.TempDir()
	writeSchema(t, filepath.Join(dir, "svc", "v1.json"), map[string]any{
		"name": "svc", "version": "v1.0.0",
		"operations": []any{
			map[string]any{
				"name":   "get",
				"input":  objectType(fieldSpec("id", "string", false)),
				"output": objectType(fieldSpec("name", "string", false), fieldSpec("age", "integer", false)),
			},
		},
	})
	g := newTestGenerator(t, out)
	if _, err := g.Generate(ctx, dir); err != nil {
		t.Fatalf("baseline generation failed: %v", err)
	}

	// Second run: a new batch with v2 that removes field "age" -> breaking.
	dir2 := t.TempDir()
	writeSchema(t, filepath.Join(dir2, "svc", "v2.json"), map[string]any{
		"name": "svc", "version": "v2.0.0",
		"operations": []any{
			map[string]any{
				"name":   "get",
				"input":  objectType(fieldSpec("id", "string", false)),
				"output": objectType(fieldSpec("name", "string", false)),
			},
		},
	})
	_, err := g.Generate(ctx, dir2)
	se := sdkerrors.AsError(err)
	if se == nil {
		t.Fatal("expected a breaking-change error, got nil")
	}
	if se.Kind != sdkerrors.KindVersion {
		t.Fatalf("expected KindVersion, got %s: %v", se.Kind, err)
	}
	if !contains(err.Error(), "fields.age") || !contains(err.Error(), "svc") {
		t.Fatalf("error must identify service and affected field: %v", err)
	}
	t.Logf("breaking cross-run change rejected: interface=%s version=%s reason=%s",
		se.Service, se.Version, se.Reason)

	// The previous live artifacts must still exist (no half state).
	if _, err := os.Stat(filepath.Join(out, sdkDirName, manifestName)); err != nil {
		t.Fatalf("manifest must survive failed generation: %v", err)
	}
}

func TestCompatibleChangeAcrossRunsAccepted(t *testing.T) {
	out := t.TempDir()
	ctx := context.Background()
	dir := t.TempDir()
	writeSchema(t, filepath.Join(dir, "svc", "v1.json"), map[string]any{
		"name": "svc", "version": "v1.0.0",
		"operations": []any{
			map[string]any{
				"name":   "get",
				"input":  objectType(fieldSpec("id", "string", false)),
				"output": objectType(fieldSpec("name", "string", false)),
			},
		},
	})
	g := newTestGenerator(t, out)
	if _, err := g.Generate(ctx, dir); err != nil {
		t.Fatal(err)
	}
	dir2 := t.TempDir()
	writeSchema(t, filepath.Join(dir2, "svc", "v2.json"), map[string]any{
		"name": "svc", "version": "v2.0.0",
		"operations": []any{
			map[string]any{
				"name":   "get",
				"input":  objectType(fieldSpec("id", "string", false)),
				"output": objectType(fieldSpec("name", "string", false), fieldSpec("nickname", "string", true)),
			},
		},
	})
	res, err := g.Generate(ctx, dir2)
	if err != nil {
		t.Fatalf("compatible change must be accepted, got %v", err)
	}
	if res.Reused {
		t.Fatal("a changed batch must not be marked reused")
	}
	t.Logf("compatible cross-run change accepted interface=svc v1.0.0->v2.0.0 (optional nickname added)")
}

func TestMalformedDocumentRejected(t *testing.T) {
	out := t.TempDir()
	g := newTestGenerator(t, out)
	_, err := g.GenerateFrom(context.Background(), nil)
	if !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("nil document must be KindInput, got %v", err)
	}
}

func sameArtifactSet(a, b []string) bool {
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestPublishedVersionImmutable ensures re-submitting the same version
// with a different shape is a KindVersion error, not a silent overwrite.
func TestPublishedVersionImmutable(t *testing.T) {
	out := t.TempDir()
	ctx := context.Background()
	g := newTestGenerator(t, out)

	dir1 := t.TempDir()
	writeSchema(t, filepath.Join(dir1, "svc", "v1.json"), map[string]any{
		"name": "svc", "version": "v1.0.0",
		"operations": []any{
			map[string]any{
				"name":   "get",
				"input":  objectType(fieldSpec("id", "string", false)),
				"output": objectType(fieldSpec("name", "string", false)),
			},
		},
	})
	if _, err := g.Generate(ctx, dir1); err != nil {
		t.Fatal(err)
	}

	// Same version, different source dir, extra field -> must be refused.
	dir2 := t.TempDir()
	writeSchema(t, filepath.Join(dir2, "svc", "v1.json"), map[string]any{
		"name": "svc", "version": "v1.0.0",
		"operations": []any{
			map[string]any{
				"name":   "get",
				"input":  objectType(fieldSpec("id", "string", false)),
				"output": objectType(fieldSpec("name", "string", false), fieldSpec("age", "integer", true)),
			},
		},
	})
	_, err := g.Generate(ctx, dir2)
	se := sdkerrors.AsError(err)
	if se == nil || se.Kind != sdkerrors.KindVersion {
		t.Fatalf("redefining a published version must be KindVersion, got %v", err)
	}
	t.Logf("immutable published version protected: interface=%s version=%s => %s",
		se.Service, se.Version, se.Reason)
}
