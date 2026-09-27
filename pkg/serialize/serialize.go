// Package serialize provides a small, pluggable serialization registry used
// both by the generator (metadata) and by the SDK runtime (wire payloads).
// Applications can register additional formats (e.g. msgpack, a custom
// binary codec) without changing the runtime core.
package serialize

import (
	"fmt"
	"sort"
	"sync"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
)

// Codec marshals and unmarshals Go values in one wire format.
type Codec interface {
	Name() string
	ContentType() string
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// Registry holds named codecs; JSON is registered by default. It is safe for
// concurrent use.
type Registry struct {
	mu     sync.RWMutex
	codecs map[string]Codec
}

// NewRegistry creates a registry pre-populated with the JSON codec.
func NewRegistry() *Registry {
	r := &Registry{codecs: map[string]Codec{}}
	r.Register(jsonCodec{})
	return r
}

// Register adds or replaces a codec by its name.
func (r *Registry) Register(c Codec) {
	if c == nil || c.Name() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codecs[c.Name()] = c
}

// Get returns a registered codec.
func (r *Registry) Get(name string) (Codec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.codecs[name]
	return c, ok
}

// Names returns all registered codec names in stable order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.codecs))
	for n := range r.codecs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Marshal encodes v with the named codec. Unknown formats and codec failures
// are normalized to categorized errors.
func (r *Registry) Marshal(name string, v any) ([]byte, error) {
	c, ok := r.Get(name)
	if !ok {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			fmt.Sprintf("unknown serialization format %q (registered: %v)", name, r.Names())).
			WithOp("serialize.marshal")
	}
	data, err := c.Marshal(v)
	if err != nil {
		return nil, apierrors.New(apierrors.CatInvocationFailed,
			fmt.Sprintf("codec %q marshal failed: %s", name, err)).
			WithOp("serialize.marshal").WithCause(err)
	}
	return data, nil
}

// Unmarshal decodes data into v with the named codec.
func (r *Registry) Unmarshal(name string, data []byte, v any) error {
	c, ok := r.Get(name)
	if !ok {
		return apierrors.New(apierrors.CatInvalidInput,
			fmt.Sprintf("unknown serialization format %q (registered: %v)", name, r.Names())).
			WithOp("serialize.unmarshal")
	}
	if err := c.Unmarshal(data, v); err != nil {
		return apierrors.New(apierrors.CatInvocationFailed,
			fmt.Sprintf("codec %q unmarshal failed: %s", name, err)).
			WithOp("serialize.unmarshal").WithCause(err)
	}
	return nil
}
