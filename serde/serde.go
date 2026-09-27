// Package serde provides pluggable serialization for generated SDK calls.
//
// The built-in JSON codec is always available. Custom codecs can be
// registered under a name and selected per interface via the "codec"
// field in an interface description. Registration failures surface as
// sdkerrors input errors and are never silent.
package serde

import (
	"fmt"
	"sort"
	"sync"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// Codec serializes and deserializes payloads.
type Codec interface {
	Name() string
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// Registry resolves codecs by name.
type Registry struct {
	mu     sync.RWMutex
	codecs map[string]Codec
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{codecs: map[string]Codec{}}
}

// Register adds a codec under its name, refusing duplicates and nil
// codecs rather than silently overwriting.
func (r *Registry) Register(c Codec) error {
	if c == nil {
		return sdkerrors.New(sdkerrors.KindInput, "cannot register nil codec")
	}
	name := c.Name()
	if name == "" {
		return sdkerrors.New(sdkerrors.KindInput, "cannot register codec with empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.codecs[name]; exists {
		return sdkerrors.Newf(sdkerrors.KindInput, "codec %q is already registered", name)
	}
	r.codecs[name] = c
	return nil
}

// Get returns the codec registered under name.
func (r *Registry) Get(name string) (Codec, error) {
	if name == "" {
		name = JSONName
	}
	r.mu.RLock()
	c, ok := r.codecs[name]
	r.mu.RUnlock()
	if !ok {
		return nil, sdkerrors.Newf(sdkerrors.KindInput,
			"no serializer registered under name %q", name)
	}
	return c, nil
}

// Names returns all registered codec names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	out := make([]string, 0, len(r.codecs))
	for n := range r.codecs {
		out = append(out, n)
	}
	r.mu.RUnlock()
	sort.Strings(out)
	return out
}

// MustDefaultRegistry returns a registry preloaded with the JSON codec.
// It panics only on a broken build (duplicate built-in registration).
func MustDefaultRegistry() *Registry {
	r := NewRegistry()
	if err := r.Register(JSONCodec{}); err != nil {
		panic(fmt.Sprintf("serde: broken built-in registry: %v", err))
	}
	return r
}
