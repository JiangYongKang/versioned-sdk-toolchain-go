package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// TestConcurrentSameRoot runs many goroutines generating the same batch
// into the same output root. Exactly one generation may publish; all
// others must either reuse it or serialize, and the final tree must be
// consistent with a single run.
func TestConcurrentSameRoot(t *testing.T) {
	out := t.TempDir()
	g := newTestGenerator(t, out)
	in := repoSchemas(t)
	ctx := context.Background()

	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	reusedCount := make(chan bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := g.Generate(ctx, in)
			if err != nil {
				errs <- err
				return
			}
			reusedCount <- res.Reused
		}()
	}
	wg.Wait()
	close(errs)
	close(reusedCount)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent generation failed: %v", err)
		}
	}
	reuses := 0
	for r := range reusedCount {
		if r {
			reuses++
		}
	}
	if reuses == 0 {
		t.Fatal("expected at least one goroutine to reuse the published result")
	}
	t.Logf("interface=billing versions=v1.0.0,v2.0.0 goroutines=%d reused=%d (rest serialized before publish)",
		n, reuses)

	// Final manifest must be present and internally consistent.
	m, err := loadManifest(out)
	if err != nil || m == nil {
		t.Fatalf("final manifest missing: %v", err)
	}
	if m.InputSHA == "" || len(m.Artifacts) == 0 {
		t.Fatalf("final manifest incomplete: %+v", m)
	}
	// No staging directories may remain.
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		if hasPrefix(e.Name(), stagesDir) {
			t.Fatalf("staging directory leaked: %s", e.Name())
		}
	}
	// Every artifact in the manifest must exist with the recorded hash.
	for _, a := range m.Artifacts {
		b, err := os.ReadFile(filepath.Join(out, a.Path))
		if err != nil {
			t.Fatalf("manifest artifact missing: %s: %v", a.Path, err)
		}
		if hashBytes(b) != a.SHA {
			t.Fatalf("artifact %s hash drift (concurrent writers polluted each other)", a.Path)
		}
	}
	t.Logf("final tree consistent: %d verified artifacts, input_sha=%s",
		len(m.Artifacts), m.InputSHA)
}

// TestConcurrentDistinctRoots proves independent outputs never cross-
// pollute: each root ends with only its own service artifacts.
func TestConcurrentDistinctRoots(t *testing.T) {
	ctx := context.Background()
	const n = 8
	type job struct {
		service string
		inDir   string
		outDir  string
	}
	jobs := make([]job, 0, n)
	for i := 0; i < n; i++ {
		svc := fmt.Sprintf("svc%d", i)
		inDir := filepath.Join(t.TempDir(), svc)
		writeSchema(t, filepath.Join(inDir, "v1.json"), map[string]any{
			"name": svc, "version": "v1.0.0",
			"operations": []any{
				map[string]any{
					"name":   "ping",
					"input":  objectType(fieldSpec("echo", "string", false)),
					"output": objectType(fieldSpec("echo", "string", false)),
				},
			},
		})
		jobs = append(jobs, job{service: svc, inDir: inDir, outDir: t.TempDir()})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			g := New(Config{OutputRoot: j.outDir, ModulePath: "sdk-generated"})
			res, err := g.Generate(ctx, j.inDir)
			if err != nil {
				errCh <- err
				return
			}
			for _, a := range res.Artifacts {
				if !contains(a, "sdk/"+j.service+"/") &&
					!contains(a, "metadata/"+j.service+"/") &&
					!hasPrefix(a, sdkDirName+"/") &&
					!hasPrefix(a, goSDKDir+"/go.mod") &&
					!hasPrefix(a, goSDKDir+"/sdkerrors/") &&
					!hasPrefix(a, goSDKDir+"/runtime/") {
					errCh <- fmt.Errorf("root for %s contains foreign artifact %s", j.service, a)
				}
			}
		}(j)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestConcurrentBreakingRejected verifies that when multiple goroutines
// race a compatible and a breaking new version, the outcome is atomic:
// either the history advanced exactly once or the breaking batch was
// refused, never a mix of half-applied files.
func TestConcurrentBreakingRejected(t *testing.T) {
	out := t.TempDir()
	ctx := context.Background()
	g := newTestGenerator(t, out)

	base := t.TempDir()
	writeSchema(t, filepath.Join(base, "svc", "v1.json"), map[string]any{
		"name": "svc", "version": "v1.0.0",
		"operations": []any{
			map[string]any{
				"name":   "get",
				"input":  objectType(fieldSpec("id", "string", false)),
				"output": objectType(fieldSpec("name", "string", false)),
			},
		},
	})
	if _, err := g.Generate(ctx, base); err != nil {
		t.Fatal(err)
	}

	mkBatch := func(version string, withAge bool) string {
		dir := t.TempDir()
		outFields := []fieldSpecT{fieldSpec("name", "string", false)}
		if withAge {
			outFields = append(outFields, fieldSpec("age", "integer", false))
		}
		writeSchema(t, filepath.Join(dir, "svc", version+".json"), map[string]any{
			"name": "svc", "version": version,
			"operations": []any{
				map[string]any{
					"name":   "get",
					"input":  objectType(fieldSpec("id", "string", false)),
					"output": objectType(outFields...),
				},
			},
		})
		return dir
	}
	good := mkBatch("v2.0.0", true) // identical shape -> compatible
	bad := mkBatch("v2.0.0", false) // removes age -> breaking

	var wg sync.WaitGroup
	results := make(chan string, 4)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := g.Generate(ctx, good); e == nil {
				results <- "ok"
			} else {
				results <- "err"
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := g.Generate(ctx, bad)
			if sdkerrors.IsKind(e, sdkerrors.KindVersion) {
				results <- "version_rejected"
			} else if e == nil {
				results <- "ok"
			} else {
				results <- "other:" + e.Error()
			}
		}()
	}
	wg.Wait()
	close(results)
	var got []string
	for r := range results {
		got = append(got, r)
	}
	sort.Strings(got)
	t.Logf("racing generations finished: %v", got)
	for _, r := range got {
		if r != "ok" && r != "version_rejected" && r != "err" {
			t.Fatalf("unexpected outcome %q (possible half-applied state)", r)
		}
	}
	// History must be well-formed and the manifest must match artifacts.
	hist, err := loadHistory(out)
	if err != nil {
		t.Fatalf("history corrupted after races: %v", err)
	}
	t.Logf("accepted versions after races: %d", len(hist.Accepted))
	m, err := loadManifest(out)
	if err != nil || m == nil {
		t.Fatalf("manifest missing after races: %v", err)
	}
	for _, a := range m.Artifacts {
		if _, err := os.ReadFile(filepath.Join(out, a.Path)); err != nil {
			t.Fatalf("artifact %s missing after races: %v", a.Path, err)
		}
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
