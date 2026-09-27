package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

type fakeBackend struct {
	out        []byte
	err        error
	gotIface   string
	gotVer     string
	gotPayload []byte
}

func (f *fakeBackend) Invoke(ctx context.Context, iface *schema.Interface, version string, payload []byte) ([]byte, error) {
	f.gotIface = iface.Name
	f.gotVer = version
	f.gotPayload = append([]byte(nil), payload...)
	return f.out, f.err
}

func sampleBundle(t *testing.T) *schema.Bundle {
	v, _ := schema.ParseVersion("1.0.0")
	return &schema.Bundle{Documents: []*schema.Document{{
		Service: "s", Version: v, Source: "t.json",
		Structs: []schema.Struct{{Name: "Pet", Fields: []schema.Field{
			{Name: "name", Type: schema.TypeRef{Primitive: schema.PrimString}},
		}}},
		Interfaces: []schema.Interface{{Name: "GetPet", Method: "M",
			Input:  []schema.Parameter{{Name: "id", Type: schema.TypeRef{Primitive: schema.PrimLong}}},
			Output: &schema.TypeRef{Name: "Pet"},
		}, {
			Name: "Ping", Method: "M2",
		}},
	}}}
}

func TestInvokeSuccessSemantics(t *testing.T) {
	be := &fakeBackend{out: []byte(`{"name":"rex"}`)}
	c, err := NewClient(sampleBundle(t), be)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Invoke(context.Background(), Request{Interface: "GetPet",
		Input: map[string]any{"id": 42}})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := resp.Output.(*map[string]any)
	if !ok {
		t.Fatalf("output type %T", resp.Output)
	}
	if (*m)["name"] != "rex" {
		t.Fatalf("success payload altered: %v", *m)
	}
	if be.gotIface != "GetPet" || be.gotVer != "1.0.0" {
		t.Fatalf("backend got iface=%s version=%s", be.gotIface, be.gotVer)
	}
	if string(be.gotPayload) != `{"id":42}` {
		t.Fatalf("encoded payload changed semantics: %s", be.gotPayload)
	}
	t.Logf("interface=GetPet version=1.0.0 verdict=success payload/response preserved")
}

func TestInvokeVoidSuccess(t *testing.T) {
	be := &fakeBackend{out: []byte(``)}
	c, _ := NewClient(sampleBundle(t), be)
	resp, err := c.Invoke(context.Background(), Request{Interface: "Ping", Input: nil})
	if err != nil || resp.Output != nil {
		t.Fatalf("void call: resp=%v err=%v", resp, err)
	}
}

func TestUnknownInterface(t *testing.T) {
	be := &fakeBackend{}
	c, _ := NewClient(sampleBundle(t), be)
	_, err := c.Invoke(context.Background(), Request{Interface: "Nope", Input: nil})
	if apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
	if be.gotIface != "" {
		t.Fatal("backend must not be called for unknown interface")
	}
	var ae *apierrors.Error
	errors.As(err, &ae)
	if ae.Interface != "Nope" {
		t.Fatalf("error not attributed to interface Nope: %v", err)
	}
	t.Logf("interface=Nope version=1.0.0 verdict=invalid_input (not dispatched)")
}

type rawErr struct{ msg string }

func (e *rawErr) Error() string { return e.msg }

func TestBackendErrorNormalized(t *testing.T) {
	be := &fakeBackend{err: &rawErr{msg: "connection reset by peer [errno 9999]"}}
	c, _ := NewClient(sampleBundle(t), be)
	_, err := c.Invoke(context.Background(), Request{Interface: "GetPet", Input: map[string]any{}})
	if apierrors.Code(err) != apierrors.CatInvocationFailed {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
	var ae *apierrors.Error
	if !errors.As(err, &ae) {
		t.Fatal("not *apierrors.Error")
	}
	if ae.Interface != "GetPet" {
		t.Fatalf("interface attribution lost: %v", ae)
	}
	// Underlying error stays available via Unwrap, never surfaced uncategorized.
	if !errors.Is(err, be.err) {
		t.Fatal("underlying backend error not wrapped")
	}
	t.Logf("interface=GetPet version=1.0.0 verdict=invocation_failed (raw error wrapped, not leaked): %v", err)
}

type versionErr struct{}

func (*versionErr) Error() string { return "" }

// backendVersionError adapts a plain transport error into the stable
// version_incompatible category to prove categorized backend errors pass
// through while raw ones are remapped.
type categorizedBackend struct{}

func (categorizedBackend) Invoke(ctx context.Context, iface *schema.Interface, version string, payload []byte) ([]byte, error) {
	return nil, apierrors.New(apierrors.CatVersionIncompatible,
		"server requires 2.0.0, client pinned to "+version).WithInterface(iface.Name)
}

func TestVersionIncompatiblePassThrough(t *testing.T) {
	c, _ := NewClient(sampleBundle(t), categorizedBackend{})
	_, err := c.Invoke(context.Background(), Request{Interface: "GetPet", Input: nil})
	if apierrors.Code(err) != apierrors.CatVersionIncompatible {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
	t.Logf("interface=GetPet version=1.0.0 verdict=version_incompatible preserved: %v", err)
}

func TestMetadataRoundTrip(t *testing.T) {
	b := sampleBundle(t)
	data := buildMetadataExported(t, b)
	c, err := NewClientFromMetadata(data, &fakeBackend{out: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if c.Version() != "1.0.0" {
		t.Fatalf("version=%s", c.Version())
	}
	if _, err := c.Invoke(context.Background(), Request{Interface: "GetPet", Input: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	t.Logf("interface=GetPet version=1.0.0 metadata round-trip verdict=invocable")
}

func TestUnknownFormat(t *testing.T) {
	_, err := NewClient(sampleBundle(t), &fakeBackend{}, WithFormat("yaml"))
	if apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
}
