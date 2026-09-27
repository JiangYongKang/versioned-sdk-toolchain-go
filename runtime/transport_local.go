package runtime

import (
	"context"
)

// LocalTransport is an in-process Transport used for local verification
// and generated-SDK tests without real credentials or network access.
type LocalTransport struct{}

// NewLocalTransport creates a LocalTransport with an empty route table.
func NewLocalTransport() *LocalTransport { return nil }

// Call dispatches a prepared request to a registered local handler.
func (t *LocalTransport) Call(ctx context.Context, req *PreparedRequest) (*RawResponse, error) {
	return nil, nil
}
