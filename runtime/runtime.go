// Package runtime is the universal client used by generated SDKs.
//
// It executes operations described by generated call metadata, applies the
// configured serialization, maps every transport/backend failure onto the
// unified sdkerrors taxonomy, and never changes the semantics of a
// successful call.
package runtime

import (
	"context"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/serde"
)

// StatusCode models a coarse backend status for error mapping.
type StatusCode int

// Backend status classes used by transports.
const (
	StatusOK          StatusCode = 200
	StatusBadRequest  StatusCode = 400
	StatusConflict    StatusCode = 409
	StatusUnavailable StatusCode = 503
)

// Request is one runtime invocation.
type Request struct {
	Service   string
	Version   string
	Operation string
	Codec     string
	Input     any
}

// Response is one runtime result.
type Response struct {
	Output any
	Raw    []byte
}

// Transport is the underlying call implementation. Backend-specific
// errors must be normalized by Normalize before reaching callers.
type Transport interface {
	Call(ctx context.Context, req *PreparedRequest) (*RawResponse, error)
}

// PreparedRequest is the serialized request handed to a transport.
type PreparedRequest struct {
	Service   string
	Version   string
	Operation string
	Body      []byte
}

// RawResponse is a transport response before decoding.
type RawResponse struct {
	Status StatusCode
	Body   []byte
}

// Client executes operations against a Transport.
type Client struct {
	transport Transport
	codecs    *serde.Registry
}

// NewClient creates a runtime client.
func NewClient(t Transport, codecs *serde.Registry) *Client { return nil }

// Invoke serializes req, calls the transport and decodes the response,
// mapping every failure to a classified sdkerrors.Error.
func (c *Client) Invoke(ctx context.Context, req *Request, out any) (*Response, error) {
	return nil, nil
}

// Normalize maps an arbitrary backend error onto sdkerrors.KindRuntime,
// preserving no foreign error surface beyond the wrapped cause.
func Normalize(op string, err error) error { return nil }

// Ensure schema import is used by skeleton consumers.
var _ = schema.KindString
