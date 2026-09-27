// Package runtime is the callable side of the generated SDK: it validates a
// request against metadata, encodes it through a registered serializer and
// normalizes every backend failure into the stable apierrors categories so
// that underlying transport details never leak to callers.
package runtime

import (
	"context"
	"fmt"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/codegen"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/serialize"
)

// Request is a runtime SDK call.
type Request struct {
	// Interface is the operation name to invoke.
	Interface string
	// Input is the caller-provided payload (encoded with the client codec).
	Input any
	// Format optionally overrides the client's serialization format.
	Format string
}

// Response is a runtime SDK result.
type Response struct {
	// Output is the decoded response payload.
	Output any
}

// Backend abstracts the underlying transport so its failures can be
// normalized. Implementations return raw bytes and opaque errors.
type Backend interface {
	Invoke(ctx context.Context, iface *schema.Interface, version string, payload []byte) ([]byte, error)
}

// Client is the SDK runtime entry point.
type Client struct {
	backend  Backend
	bundle   *schema.Bundle
	registry *serialize.Registry
	format   string
	version  string
}

// Option customizes a Client.
type Option func(*Client)

// WithFormat selects a registered serialization format (default "json").
func WithFormat(name string) Option {
	return func(c *Client) { c.format = name }
}

// WithRegistry injects a registry carrying custom codecs.
func WithRegistry(r *serialize.Registry) Option {
	return func(c *Client) { c.registry = r }
}

// NewClient builds a runtime client bound to a validated bundle and backend.
func NewClient(b *schema.Bundle, backend Backend, opts ...Option) (*Client, error) {
	if b == nil || len(b.Documents) == 0 {
		return nil, apierrors.New(apierrors.CatInvalidInput, "client requires a non-empty bundle").
			WithOp("runtime.new")
	}
	if backend == nil {
		return nil, apierrors.New(apierrors.CatInvalidInput, "client requires a backend").
			WithOp("runtime.new")
	}
	c := &Client{
		backend:  backend,
		bundle:   b,
		registry: serialize.NewRegistry(),
		format:   "json",
		version:  b.Documents[0].Version.String(),
	}
	for _, o := range opts {
		o(c)
	}
	if _, ok := c.registry.Get(c.format); !ok {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			fmt.Sprintf("serialization format %q is not registered", c.format)).
			WithOp("runtime.new")
	}
	return c, nil
}

// NewClientFromMetadata builds a bundle from generated metadata bytes. It is
// a convenience for hosts that ship only the metadata.json artifact.
func NewClientFromMetadata(data []byte, backend Backend, opts ...Option) (*Client, error) {
	var md codegen.Metadata
	if err := decodeMetadata(data, &md); err != nil {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			"invalid metadata: "+err.Error()).WithOp("runtime.new").WithCause(err)
	}
	b := bundleFromMetadata(&md)
	return NewClient(b, backend, opts...)
}

// Version returns the pinned API version of this client.
func (c *Client) Version() string { return c.version }

// Invoke performs one SDK call.
//
// Errors always carry one of: invalid_input (unknown interface, bad format,
// encoding failure), version_incompatible (backend negotiated a different
// version), invocation_failed (transport/decode failure). Successful calls
// return the decoded response untouched.
func (c *Client) Invoke(ctx context.Context, req Request) (*Response, error) {
	iface, _ := c.bundle.FindInterface(req.Interface)
	if iface == nil {
		return nil, apierrors.New(apierrors.CatInvalidInput,
			fmt.Sprintf("unknown interface %q", req.Interface)).
			WithOp("runtime.invoke").WithInterface(req.Interface)
	}
	format := req.Format
	if format == "" {
		format = c.format
	}
	payload, err := c.registry.Marshal(format, req.Input)
	if err != nil {
		// Registry already categorizes; pin interface context.
		return nil, withInterface(err, req.Interface)
	}
	out, err := c.backend.Invoke(ctx, iface, c.version, payload)
	if err != nil {
		return nil, normalizeBackendError(err, req.Interface)
	}
	resp := &Response{}
	if iface.Output != nil {
		target := newOutputTarget(*iface.Output)
		if err := c.registry.Unmarshal(format, out, target); err != nil {
			return nil, withInterface(err, req.Interface)
		}
		resp.Output = target
	} else {
		resp.Output = nil
	}
	return resp, nil
}

// normalizeBackendError maps arbitrary backend failures onto stable
// categories without leaking backend-specific error strings beyond the
// wrapped cause.
func normalizeBackendError(err error, iface string) error {
	switch apierrors.Code(err) {
	case apierrors.CatVersionIncompatible, apierrors.CatInvalidInput,
		apierrors.CatGenerationFailed:
		return err
	}
	return apierrors.New(apierrors.CatInvocationFailed,
		"backend invocation failed: "+err.Error()).
		WithOp("runtime.invoke").WithInterface(iface).WithCause(err)
}

func withInterface(err error, iface string) error {
	var ae *apierrors.Error
	if asError(err, &ae) {
		return ae.WithInterface(iface)
	}
	return apierrors.New(apierrors.Code(err), err.Error()).
		WithOp("runtime.invoke").WithInterface(iface).WithCause(err)
}
