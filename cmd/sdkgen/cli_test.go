package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildCLI compiles the sdkgen binary once per test process.
func buildCLI(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	bin := filepath.Join(t.TempDir(), "sdkgen")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(joinRepo(t), "cmd", "sdkgen")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build sdkgen: %v\n%s", err, out)
	}
	return bin
}

func joinRepo(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func repoSchemas(t *testing.T) string {
	return filepath.Join(joinRepo(t), "testdata", "schemas")
}

func runCLI(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return string(out), code
}

func TestCLIValidateAcceptsCompatibleFixtures(t *testing.T) {
	bin := buildCLI(t)
	out, code := runCLI(t, bin, "validate", "--in", repoSchemas(t))
	if code != 0 {
		t.Fatalf("validate exit=%d output=%s", code, out)
	}
	if !strings.Contains(out, "COMPATIBLE") || !strings.Contains(out, "enum value added") {
		t.Fatalf("validate output must print version and basis:\n%s", out)
	}
	t.Logf("CLI verdict output:\n%s", out)
}

func TestCLIGenerateIsDeterministic(t *testing.T) {
	bin := buildCLI(t)
	dst := t.TempDir()

	first, code := runCLI(t, bin, "generate",
		"--in", repoSchemas(t), "--out", dst, "--module", "sdk-generated")
	if code != 0 {
		t.Fatalf("first generate exit=%d: %s", code, first)
	}
	if !strings.Contains(first, "reused=false") {
		t.Fatalf("first run must not be reused:\n%s", first)
	}

	second, code := runCLI(t, bin, "generate",
		"--in", repoSchemas(t), "--out", dst, "--module", "sdk-generated")
	if code != 0 {
		t.Fatalf("second generate exit=%d: %s", code, second)
	}
	if !strings.Contains(second, "reused=true") {
		t.Fatalf("second run must reuse:\n%s", second)
	}
	t.Logf("second run output:\n%s", second)
}

func TestCLIBreakingInputExitsThree(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	v1 := `{"name":"svc","version":"v1.0.0","operations":[
		{"name":"get","input":{"kind":"object","fields":[
		 {"name":"id","type":{"kind":"string"}}]},
		 "output":{"kind":"object","fields":[
		 {"name":"name","type":{"kind":"string"}},
		 {"name":"age","type":{"kind":"integer"}}]}}]}`
	v2 := `{"name":"svc","version":"v2.0.0","operations":[
		{"name":"get","input":{"kind":"object","fields":[
		 {"name":"id","type":{"kind":"string"}}]},
		 "output":{"kind":"object","fields":[
		 {"name":"name","type":{"kind":"string"}}]}}]}`
	d1 := filepath.Join(dir, "a")
	d2 := filepath.Join(dir, "b")
	mustWrite(t, filepath.Join(d1, "svc", "v1.json"), v1)
	mustWrite(t, filepath.Join(d2, "svc", "v2.json"), v2)
	out1 := t.TempDir()
	_, code := runCLI(t, bin, "generate", "--in", d1, "--out", out1)
	if code != 0 {
		t.Fatalf("baseline generation exit=%d", code)
	}
	// Second generation into the same out root with breaking v2.
	out2, code := runCLI(t, bin, "generate", "--in", d2, "--out", out1)
	if code != 3 {
		t.Fatalf("breaking change must exit 3, got %d: %s", code, out2)
	}
	if !strings.Contains(out2, "fields.age") {
		t.Fatalf("exit output must name affected field:\n%s", out2)
	}
	t.Logf("breaking exit code=3 output:\n%s", out2)
}

func TestCLIMalformedExitsTwo(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "svc", "v1.json"), `{"name":"svc", broken json`)
	out, code := runCLI(t, bin, "validate", "--in", dir)
	if code != 2 {
		t.Fatalf("malformed input must exit 2, got %d: %s", code, out)
	}
	t.Logf("malformed exit code=2: %s", strings.TrimSpace(out))
}

func TestCLIMissingArgs(t *testing.T) {
	bin := buildCLI(t)
	_, code := runCLI(t, bin, "generate")
	if code != 2 {
		t.Fatalf("missing args must exit 2, got %d", code)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
