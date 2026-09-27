package serde

import (
	"bytes"
	"encoding/json"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// JSONName is the built-in codec name and the default for interfaces
// that omit the codec field.
const JSONName = "json"

// JSONCodec is the deterministic built-in JSON serializer.
type JSONCodec struct{}

// Name implements Codec.
func (JSONCodec) Name() string { return JSONName }

// Marshal serializes v with stable (sorted-key) JSON. Maps are emitted
// with sorted keys regardless of the standard library's Go 1.12+
// behavior, by routing through json.Indent after compacting.
func (JSONCodec) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, sdkerrors.New(sdkerrors.KindRuntime,
			"serialization failed while encoding request payload").Wrap(err)
	}
	// Encode appends a trailing newline; remove it for byte-stable output.
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return out, nil
}

// Unmarshal decodes data, rejecting trailing content and unknown fields.
func (JSONCodec) Unmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(v); err != nil {
		return sdkerrors.New(sdkerrors.KindRuntime,
			"serialization failed while decoding response payload").Wrap(err)
	}
	if dec.More() {
		return sdkerrors.New(sdkerrors.KindRuntime,
			"response payload contains trailing data after the first JSON value")
	}
	return nil
}
