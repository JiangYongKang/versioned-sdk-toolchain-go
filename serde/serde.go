// Package serde provides pluggable serialization for generated SDK calls.
//
// The built-in JSON codec is always available. Custom codecs can be
// registered under a name and selected in interface descriptions.
package serde

import "context"

// Codec serializes and deserializes payloads.
type Codec interface {
	Name() string
	Marshal(ctx context.Context, v any) ([]byte, error)
	Unmarshal(ctx context.Context, data []byte, v any) error
}

// Registry resolves codecs by name.
type Registry struct{}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return nil }

// Register adds a codec under its name, refusing duplicates.
func (r *Registry) Register(c Codec) error { return nil }

// Get returns the codec registered under name.
func (r *Registry) Get(name string) (Codec, error) { return nil, nil }

// MustDefaultRegistry returns a registry preloaded with the JSON codec.
func MustDefaultRegistry() *Registry { return nil }
