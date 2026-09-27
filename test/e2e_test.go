// Package test contains end-to-end tests that exercise the whole local
// toolchain: load -> validate -> generate -> runtime invoke, plus
// cross-version compatibility and concurrent repeated generation.
package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/codegen"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/compat"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/load"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/runtime"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

const v1Doc = `{
  "service": "shop", "version": "1.0.0",
  "enums": [{"name": "Status", "values": ["open", "closed"]}],
  "structs": [{
    "name": "Order",
    "fields": [
      {"name": "id", "type": {"primitive": "long"}},
      {"name": "status", "type": {"ref": "Status"}},
      {"name": "note", "type": {"primitive": "string"}, "optional": true}
    ]
  }],
  "interfaces": [{
    "name": "GetOrder", "method": "GET /orders/{id}",
    "input": [{"name": "id", "type": {"primitive": "long"}}],
    "output": {"ref": "Order"}
  }]
}`

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

func loadBundle(t *testing.T, path string) *schema.Bundle {
	t.Helper()
	b, err := load.New().LoadFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEndToEndGenerateCompileAndInvoke(t *testing.T) {
	root := t.TempDir()
	src := writeFile(t, root, "descriptions/v1.json", v1Doc)
	b := loadBundle(t, src)

	out := filepath.Join(root, "sdk")
	res, err := codegen.Generate(b, codegen.Options{OutputDir: out, Package: "shopsdk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Artifacts) != 6 {
		t.Fatalf("artifacts=%v", res.Artifacts)
	}

	// The generated package must compile.
	modDir := filepath.Join(root, "genmodule")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sdkDir := filepath.Join(modDir, "shopsdk")
	if err := copyTree(out, sdkDir); err != nil {
		t.Fatal(err)
	}
	writeGoMod(t, modDir)
	cmd := exec.Command("go", "build", "./shopsdk")
	cmd.Dir = modDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated SDK failed to compile: %v\n%s", err, out)
	}

	// Metadata-driven runtime invocation keeps success semantics.
	be := &scriptedBackend{response: []byte(`{"id":7,"status":"open","note":"hi"}`)}
	client, err := runtime.NewClientFromMetadata(res.Metadata, be)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Invoke(context.Background(), runtime.Request{
		Interface: "GetOrder", Input: map[string]any{"id": 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := (*(resp.Output.(*map[string]any)))
	if m["status"] != "open" || m["note"] != "hi" {
		t.Fatalf("success response semantics changed: %v", m)
	}
	t.Logf("interface=GetOrder version=1.0.0 e2e verdict=generated+compiled+invoked payload=%s",
		be.lastPayload)
}

func TestEndToEndCrossVersion(t *testing.T) {
	root := t.TempDir()
	oldP := writeFile(t, root, "v1.json", v1Doc)

	compatible := strings.Replace(v1Doc, `"1.0.0"`, `"1.1.0"`, 1)
	compatible = strings.Replace(compatible, `"open", "closed"`, `"open", "closed", "pending"`, 1)
	compatible = strings.Replace(compatible,
		`{"name": "note", "type": {"primitive": "string"}, "optional": true}`,
		`{"name": "note", "type": {"primitive": "string"}, "optional": true},
       {"name": "coupon", "type": {"primitive": "string"}, "optional": true}`, 1)
	newP := writeFile(t, root, "v2.json", compatible)

	report, err := compat.Compare(loadBundle(t, oldP), loadBundle(t, newP))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Compatible() {
		t.Fatalf("expected compatible, got breaking: %v", report.Breaking())
	}
	t.Logf("interface=GetOrder version=1.0.0->1.1.0 e2e verdict=compatible (%d changes)", len(report.Changes))

	// A breaking variant removing a field must be rejected with context.
	breaking := strings.Replace(v1Doc, `"1.0.0"`, `"2.0.0"`, 1)
	breaking = strings.Replace(breaking,
		`{"name": "status", "type": {"ref": "Status"}},`, "", 1)
	bp := writeFile(t, root, "v2break.json", breaking)
	report2, err := compat.Compare(loadBundle(t, oldP), loadBundle(t, bp))
	if err != nil {
		t.Fatal(err)
	}
	if report2.Compatible() {
		t.Fatal("expected breaking report")
	}
	found := false
	for _, c := range report2.Breaking() {
		if c.Field == "Order.status" && strings.Contains(c.Reason, "field removed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Order.status removal not reported: %v", report2.Breaking())
	}
	// Generation against the breaking schema itself still works (the two
	// versions simply cannot be promoted compatibly), and codegen of v2
	// must fail generation against the v1 output only if explicitly gated.
	t.Logf("interface=GetOrder version=1.0.0->2.0.0 e2e verdict=breaking, affected=%s",
		report2.Breaking()[0].Field)
}

func TestEndToEndMalformedRejected(t *testing.T) {
	root := t.TempDir()
	bad := strings.Replace(v1Doc, `{"ref": "Status"}`, `{"ref": "NoSuchStatus"}`, 1)
	p := writeFile(t, root, "bad.json", bad)
	b := loadBundle(t, p)
	_, err := codegen.Generate(b, codegen.Options{OutputDir: filepath.Join(root, "out")})
	if apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
	if !strings.Contains(err.Error(), "Order.status") {
		t.Fatalf("missing field location: %v", err)
	}
	t.Logf("interface=<struct Order.status> e2e verdict=invalid_input, generation refused:\n%v", err)
}

func TestEndToEndConcurrentRepeated(t *testing.T) {
	root := t.TempDir()
	b := loadBundle(t, writeFile(t, root, "v1.json", v1Doc))
	out := filepath.Join(root, "sdk")

	const workers, rounds = 6, 4
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string
	var first []byte
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				res, err := codegen.Generate(b, codegen.Options{OutputDir: out, Package: "sdk"})
				if err != nil {
					mu.Lock()
					failures = append(failures, err.Error())
					mu.Unlock()
					return
				}
				mu.Lock()
				if first == nil {
					first = append([]byte(nil), res.Metadata...)
				} else if string(first) != string(res.Metadata) {
					failures = append(failures, "metadata diverged across concurrent runs")
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	if len(failures) != 0 {
		t.Fatalf("concurrent generation failures: %v", failures[:min(3, len(failures))])
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 6 {
		t.Fatalf("leftover/partial artifacts: %v", entries)
	}
	t.Logf("interface=GetOrder version=1.0.0 workers=%d rounds=%d verdict=stable no pollution",
		workers, rounds)
}

type scriptedBackend struct {
	response    []byte
	lastPayload []byte
}

func (s *scriptedBackend) Invoke(ctx context.Context, iface *schema.Interface, version string, payload []byte) ([]byte, error) {
	s.lastPayload = append([]byte(nil), payload...)
	return s.response, nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func writeGoMod(t *testing.T, dir string) {
	t.Helper()
	content := "module example.com/genmodule\n\ngo 1.25.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
