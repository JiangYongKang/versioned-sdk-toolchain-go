package serialize

import (
	"strings"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
)

// upperCodec is a demo extension: it only supports strings and emits them
// upper-cased with a marker prefix.
type upperCodec struct{}

type upperTypeError struct{}

func (e upperTypeError) Error() string { return "upper codec only supports strings" }

var errUpperType error = upperTypeError{}

func (upperCodec) Name() string        { return "upper" }
func (upperCodec) ContentType() string { return "text/x-upper" }
func (upperCodec) Marshal(v any) ([]byte, error) {
	s, ok := v.(string)
	if !ok {
		return nil, errUpperType
	}
	return []byte("UPPER:" + strings.ToUpper(s)), nil
}
func (upperCodec) Unmarshal(data []byte, v any) error {
	p, ok := v.(*string)
	if !ok {
		return errUpperType
	}
	*p = strings.ToLower(strings.TrimPrefix(string(data), "UPPER:"))
	return nil
}

func TestRegistryExtension(t *testing.T) {
	r := NewRegistry()
	r.Register(upperCodec{})
	data, err := r.Marshal("upper", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "UPPER:HELLO" {
		t.Fatalf("custom codec output=%q", data)
	}
	var got string
	if err := r.Unmarshal("upper", data, &got); err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("round trip=%q", got)
	}
}

func TestRegistryUnknownFormat(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Marshal("yaml", nil); apierrors.Code(err) != apierrors.CatInvalidInput {
		t.Fatalf("category=%s", apierrors.Code(err))
	}
}

func TestRegistryCodecFailureNormalized(t *testing.T) {
	r := NewRegistry()
	r.Register(upperCodec{})
	_, err := r.Marshal("upper", 123)
	if apierrors.Code(err) != apierrors.CatInvocationFailed {
		t.Fatalf("category=%s err=%v", apierrors.Code(err), err)
	}
	t.Logf("serializer=upper verdict=invocation_failed on codec-level failure: %v", err)
}

func TestJSONDefault(t *testing.T) {
	r := NewRegistry()
	found := false
	for _, n := range r.Names() {
		if n == "json" {
			found = true
		}
	}
	if !found {
		t.Fatal("json codec must be registered by default")
	}
	data, err := r.Marshal("json", map[string]int{"a": 1})
	if err != nil || string(data) != `{"a":1}` {
		t.Fatalf("json marshal: %s %v", data, err)
	}
}
