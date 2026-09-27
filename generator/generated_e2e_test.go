package generator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGeneratedSDKEndToEnd generates the billing SDK, drops a test file
// into the generated module and runs "go test" on it. This proves the
// emitted typed client preserves success semantics and that backend
// failures normalize to the generated sdkerrors taxonomy.
func TestGeneratedSDKEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	out := t.TempDir()
	g := newTestGenerator(t, out)
	if _, err := g.Generate(context.Background(), repoSchemas(t)); err != nil {
		t.Fatal(err)
	}

	pkgDir := filepath.Join(out, "go", "sdk", "billing", "v2_0_0")
	testSrc := `package v2_0_0

import (
	"context"
	"testing"

	"sdk-generated/runtime"
	"sdk-generated/sdkerrors"
)

func TestGeneratedCreateInvoiceSuccess(t *testing.T) {
	lt := runtime.NewLocalTransport()
	lt.Handle("billing", "create_invoice", func(ctx context.Context, req *runtime.PreparedRequest) (*runtime.RawResponse, error) {
		if string(req.Body) != ` + "`" + `{"customer_id":"c1","total":{"amount_minor":500,"currency":"USD"}}` + "`" + ` {
			t.Fatalf("unexpected wire body: %s", req.Body)
		}
		return &runtime.RawResponse{Status: runtime.StatusOK, Body: []byte(` + "`" + `{"invoice_id":"inv-9","status":"open"}` + "`" + `)}, nil
	})
	c := NewClient(runtime.NewClient(lt))
	out, err := c.CreateInvoice(context.Background(), &CreateInvoiceInput{
		CustomerId: "c1",
		Total:      Money{AmountMinor: 500, Currency: Currency("USD")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.InvoiceId != "inv-9" || out.Status != "open" {
		t.Fatalf("success semantics changed: %+v", out)
	}
}

func TestGeneratedBackendFailureNormalized(t *testing.T) {
	c := NewClient(runtime.NewClient(runtime.NewFailingTransport("native")))
	_, err := c.ListInvoices(context.Background(), &ListInvoicesInput{CustomerId: "c1"})
	if err == nil {
		t.Fatal("expected normalized runtime error")
	}
	if !sdkerrors.IsKind(err, sdkerrors.KindRuntime) {
		t.Fatalf("foreign backend error leaked unnormalized: %v", err)
	}
}

func TestGeneratedBadRequestMapping(t *testing.T) {
	lt := runtime.NewLocalTransport()
	lt.Handle("billing", "get_invoice", func(ctx context.Context, req *runtime.PreparedRequest) (*runtime.RawResponse, error) {
		return &runtime.RawResponse{Status: runtime.StatusBadRequest, Body: nil}, nil
	})
	c := NewClient(runtime.NewClient(lt))
	_, err := c.GetInvoice(context.Background(), &GetInvoiceInput{InvoiceId: "?"})
	if !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("400 must map to input_error, got %v", err)
	}
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "zz_e2e_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "test", "./sdk/billing/v2_0_0/", "-v")
	cmd.Dir = filepath.Join(out, "go")
	gocache := t.TempDir()
	cmd.Env = append(os.Environ(), "GO111MODULE=on", "GOCACHE="+gocache, "GOPATH="+t.TempDir())
	res, err := cmd.CombinedOutput()
	t.Logf("generated module test output:\n%s", res)
	if err != nil {
		t.Fatalf("generated SDK tests failed: %v", err)
	}
}
