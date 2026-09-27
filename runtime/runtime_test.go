package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/serde"
)

func echoTransport(status StatusCode, body []byte, err error) Transport {
	return TransportFunc(func(_ context.Context, req *PreparedRequest) (*RawResponse, error) {
		if err != nil {
			return nil, err
		}
		return &RawResponse{Status: status, Body: body}, nil
	})
}

// TransportFunc adapts a function into a Transport.
type TransportFunc func(ctx context.Context, req *PreparedRequest) (*RawResponse, error)

func (f TransportFunc) Call(ctx context.Context, req *PreparedRequest) (*RawResponse, error) {
	return f(ctx, req)
}

type reqBody struct {
	N int `json:"n"`
}
type respBody struct {
	Doubled int `json:"doubled"`
}

func TestInvokeSuccessSemanticsUnchanged(t *testing.T) {
	tr := TransportFunc(func(_ context.Context, req *PreparedRequest) (*RawResponse, error) {
		// Success path: echo back based on input, status 200.
		if string(req.Body) != `{"n":21}` {
			t.Errorf("serialized request changed: %s", req.Body)
		}
		return &RawResponse{Status: StatusOK, Body: []byte(`{"doubled":42}`)}, nil
	})
	c := NewClient(tr, serde.MustDefaultRegistry())
	var out respBody
	res, err := c.Invoke(context.Background(), &Request{
		Service: "svc", Version: "v1", Operation: "double",
		Input: reqBody{N: 21},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Doubled != 42 {
		t.Fatalf("decoded output semantics changed: %+v", out)
	}
	if string(res.Raw) != `{"doubled":42}` {
		t.Fatalf("raw response changed: %s", res.Raw)
	}
	t.Logf("interface=svc version=v1 op=double basis=200+exact payload round-trip: out=%+v", out)
}

func TestBackendStatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		status StatusCode
		kind   sdkerrors.Kind
		basis  string
	}{
		{"bad request maps to input error", StatusBadRequest, sdkerrors.KindInput, "400"},
		{"conflict maps to version error", StatusConflict, sdkerrors.KindVersion, "409"},
		{"unavailable maps to runtime error", StatusUnavailable, sdkerrors.KindRuntime, "503"},
		{"other 5xx maps to runtime error", StatusCode(502), sdkerrors.KindRuntime, "502"},
		{"unknown 4xx maps to runtime error", StatusCode(418), sdkerrors.KindRuntime, "418"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(echoTransport(tc.status, nil, nil), serde.MustDefaultRegistry())
			_, err := c.Invoke(context.Background(), &Request{
				Service: "svc", Version: "v1", Operation: "op", Input: reqBody{N: 1},
			}, nil)
			se := sdkerrors.AsError(err)
			if se == nil || se.Kind != tc.kind {
				t.Fatalf("status %s: got %v want kind %s", tc.basis, err, tc.kind)
			}
			if se.Op != "svc.op" {
				t.Fatalf("op annotation missing: %+v", se)
			}
			t.Logf("interface=svc version=v1 op=op status=%s => kind=%s basis=%s",
				tc.basis, se.Kind, se.Reason)
		})
	}
}

func TestForeignErrorNormalized(t *testing.T) {
	c := NewClient(NewFailingTransport("native-driver"), serde.MustDefaultRegistry())
	_, err := c.Invoke(context.Background(), &Request{
		Service: "svc", Version: "v1", Operation: "boom", Input: reqBody{N: 1},
	}, nil)
	se := sdkerrors.AsError(err)
	if se == nil || se.Kind != sdkerrors.KindRuntime {
		t.Fatalf("foreign vendor error must normalize to KindRuntime, got %v", err)
	}
	// Stable surface uses our reason; foreign text is only the wrapped cause.
	if strings.Contains(se.Reason, "vendor-sdk") {
		t.Fatalf("foreign error text leaked into stable reason: %q", se.Reason)
	}
	if !errors.Is(err, &vendorPanicError{}) {
		// errors.Is on pointer target works only for same pointer; the
		// real check is substring of the full chain below.
	}
	if !strings.Contains(err.Error(), "vendor-sdk<native-driver>") {
		t.Fatalf("wrapped cause should retain detail for diagnostics: %v", err)
	}
	t.Logf("interface=svc version=v1 op=boom normalized=%v", err)
}

func TestUnknownCodecIsInputError(t *testing.T) {
	c := NewClient(echoTransport(StatusOK, []byte(`{}`), nil), serde.MustDefaultRegistry())
	_, err := c.Invoke(context.Background(), &Request{
		Service: "svc", Operation: "op", Codec: "msgpack", Input: reqBody{},
	}, nil)
	if !sdkerrors.IsKind(err, sdkerrors.KindInput) {
		t.Fatalf("unknown codec must be KindInput, got %v", err)
	}
}

func TestContextCanceledNormalized(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewClient(echoTransport(StatusOK, nil, nil), serde.MustDefaultRegistry())
	_, err := c.Invoke(ctx, &Request{Service: "svc", Operation: "op", Input: reqBody{}}, nil)
	if !sdkerrors.IsKind(err, sdkerrors.KindRuntime) {
		t.Fatalf("canceled context must map to KindRuntime, got %v", err)
	}
}

func TestLocalTransportRouting(t *testing.T) {
	lt := NewLocalTransport()
	lt.Handle("svc", "ping", func(_ context.Context, req *PreparedRequest) (*RawResponse, error) {
		return &RawResponse{Status: StatusOK, Body: req.Body}, nil
	})
	c := NewClient(lt, serde.MustDefaultRegistry())
	var out map[string]any
	_, err := c.Invoke(context.Background(), &Request{
		Service: "svc", Operation: "ping", Input: map[string]any{"ok": true},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Fatalf("local echo semantics changed: %#v", out)
	}

	// Missing route: the local transport's concrete error must normalize.
	_, err = c.Invoke(context.Background(), &Request{
		Service: "svc", Operation: "missing", Input: map[string]any{},
	}, nil)
	if !sdkerrors.IsKind(err, sdkerrors.KindRuntime) {
		t.Fatalf("missing route must be KindRuntime, got %v", err)
	}
	t.Logf("missing route normalized: %v", err)
}

func TestNormalizeNil(t *testing.T) {
	if err := Normalize("x", nil); err != nil {
		t.Fatalf("nil error must normalize to nil, got %v", err)
	}
	if err := NormalizeStatus("x", StatusOK, nil); err != nil {
		t.Fatalf("200 must normalize to nil, got %v", err)
	}
	if got := NormalizeStatus("x", StatusBadRequest, fmt.Errorf("e")); !sdkerrors.IsKind(got, sdkerrors.KindInput) {
		t.Fatalf("400 must be KindInput, got %v", got)
	}
}
