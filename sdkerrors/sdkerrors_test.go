package sdkerrors

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorFormattingIsStable(t *testing.T) {
	e := New(KindVersion, "breaking change rejected").
		WithService("billing").
		WithVersion("v1.0.0->v2.0.0").
		WithOp("get_invoice").
		WithField("output.fields.total").
		Wrap(errors.New("type kind changed: object -> string"))

	want := "version_incompatible: breaking change rejected " +
		"[service=billing, version=v1.0.0->v2.0.0, op=get_invoice, " +
		"field=output.fields.total]: type kind changed: object -> string"
	if got := e.Error(); got != want {
		t.Fatalf("message mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestIsKindAndAsError(t *testing.T) {
	base := New(KindInput, "bad enum")
	wrapped := fmt.Errorf("load failed: %w", base)

	if !IsKind(wrapped, KindInput) {
		t.Fatal("expected wrapped error to classify as KindInput")
	}
	if IsKind(wrapped, KindRuntime) {
		t.Fatal("did not expect KindRuntime")
	}
	if IsKind(nil, KindInput) {
		t.Fatal("nil must not classify as any kind")
	}
	se := AsError(wrapped)
	if se == nil || se.Reason != "bad enum" {
		t.Fatalf("AsError did not recover structured error: %#v", se)
	}
	if AsError(errors.New("plain")) != nil {
		t.Fatal("plain error should not convert")
	}
}

func TestChainedWrappingUnwraps(t *testing.T) {
	foreign := errors.New("foreign vendor failure")
	e := New(KindRuntime, "normalized").Wrap(foreign)
	if !errors.Is(e, foreign) {
		t.Fatal("wrapped cause must remain reachable via Unwrap")
	}
}
