// Package runtime is the universal client used by generated SDKs.
//
// It executes operations described by generated call metadata, applies
// the configured serialization, maps every transport/backend failure
// onto the unified sdkerrors taxonomy, and never changes the semantics of
// a successful call (the decoded response payload is handed back
// untouched).
package runtime

import (
	"context"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/serde"
)

// StatusCode models a coarse backend status for error mapping.
type StatusCode int

// Backend status classes understood by the normalizer.
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

// PreparedRequest is the serialized request handed to a transport.
type PreparedRequest struct {
	Service   string
	Version   string
	Operation string
	Codec     string
	Body      []byte
}

// RawResponse is a transport response before decoding.
type RawResponse struct {
	Status StatusCode
	Body   []byte
}

// Transport is the underlying call implementation. Backend-specific
// errors are mapped to the stable taxonomy by the Client before callers
// ever see them.
type Transport interface {
	Call(ctx context.Context, req *PreparedRequest) (*RawResponse, error)
}

// Client executes operations against a Transport.
type Client struct {
	transport Transport
	codecs    *serde.Registry
}

// NewClient creates a runtime client.
func NewClient(t Transport, codecs *serde.Registry) *Client {
	if codecs == nil {
		codecs = serde.MustDefaultRegistry()
	}
	return &Client{transport: t, codecs: codecs}
}

// Invoke serializes req, calls the transport and decodes the response,
// mapping every failure to a classified sdkerrors.Error.
func (c *Client) Invoke(ctx context.Context, req *Request, out any) (*Response, error) {
	if req == nil {
		return nil, sdkerrors.New(sdkerrors.KindInput, "nil invocation request")
	}
	if c.transport == nil {
		return nil, sdkerrors.New(sdkerrors.KindRuntime, "client has no transport configured")
	}
	op := req.Service + "." + req.Operation

	codecName := req.Codec
	if codecName == "" {
		codecName = serde.JSONName
	}
	codec, err := c.codecs.Get(codecName)
	if err != nil {
		return nil, sdkerrors.AsError(err).WithOp(op)
	}

	if err := ctx.Err(); err != nil {
		return nil, Normalize(op, err)
	}

	body, err := codec.Marshal(req.Input)
	if err != nil {
		// A non-encodable request is a caller/input problem.
		if se := sdkerrors.AsError(err); se != nil {
			return nil, se.WithOp(op).WithService(req.Service).WithVersion(req.Version)
		}
		return nil, sdkerrors.New(sdkerrors.KindInput,
			"request payload could not be serialized").WithOp(op).Wrap(err)
	}

	raw, err := c.transport.Call(ctx, &PreparedRequest{
		Service:   req.Service,
		Version:   req.Version,
		Operation: req.Operation,
		Codec:     codecName,
		Body:      body,
	})
	if err != nil {
		return nil, Normalize(op, err)
	}
	if raw == nil {
		return nil, sdkerrors.New(sdkerrors.KindRuntime,
			"transport returned a nil response without an error").WithOp(op)
	}
	if nerr := NormalizeStatus(op, raw.Status, nil); nerr != nil {
		return nil, nerr
	}
	if out != nil {
		if err := codec.Unmarshal(raw.Body, out); err != nil {
			return nil, sdkerrors.AsError(err).WithOp(op).
				WithService(req.Service).WithVersion(req.Version)
		}
	}
	return &Response{Output: out, Raw: raw.Body}, nil
}

// Normalize maps an arbitrary transport/backend error onto the stable
// taxonomy: context problems remain runtime-call failures, everything
// foreign becomes KindRuntime with the original retained only as cause.
func Normalize(op string, err error) error {
	if err == nil {
		return nil
	}
	return sdkerrors.New(sdkerrors.KindRuntime,
		"runtime call to the underlying backend failed").WithOp(op).Wrap(err)
}

// NormalizeStatus maps a transport status (with optional cause) to the
// taxonomy. This is where different backends converge to one surface.
func NormalizeStatus(op string, status StatusCode, cause error) error {
	switch {
	case status == StatusOK:
		return nil
	case status == StatusBadRequest:
		return sdkerrors.New(sdkerrors.KindInput,
			"backend rejected the request as invalid").WithOp(op).Wrap(cause)
	case status == StatusConflict:
		return sdkerrors.New(sdkerrors.KindVersion,
			"backend reported a version conflict").WithOp(op).Wrap(cause)
	case status == StatusUnavailable, status >= 500:
		return sdkerrors.New(sdkerrors.KindRuntime,
			"backend is unavailable or failed internally").WithOp(op).Wrap(cause)
	default:
		return sdkerrors.Newf(sdkerrors.KindRuntime,
			"backend returned unmapped status %d", int(status)).
			WithOp(op).Wrap(cause)
	}
}
