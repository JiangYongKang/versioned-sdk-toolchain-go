package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// HandlerFunc is one local operation implementation. It receives the
// serialized request body and returns a raw response.
type HandlerFunc func(ctx context.Context, req *PreparedRequest) (*RawResponse, error)

// LocalTransport is an in-process Transport used for local verification
// and generated-SDK tests without real credentials or network access.
type LocalTransport struct {
	mu       sync.RWMutex
	routes   map[string]HandlerFunc
	fallback HandlerFunc
}

// NewLocalTransport creates a LocalTransport with an empty route table.
func NewLocalTransport() *LocalTransport {
	return &LocalTransport{routes: map[string]HandlerFunc{}}
}

// Handle registers a handler for "service.operation".
func (t *LocalTransport) Handle(service, operation string, h HandlerFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes[routeKey(service, operation)] = h
}

// SetFallback installs a handler invoked when no explicit route matches.
func (t *LocalTransport) SetFallback(h HandlerFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fallback = h
}

// Routes lists registered "service.operation" keys deterministically.
func (t *LocalTransport) Routes() []string {
	t.mu.RLock()
	out := make([]string, 0, len(t.routes))
	for k := range t.routes {
		out = append(out, k)
	}
	t.mu.RUnlock()
	sort.Strings(out)
	return out
}

// Call dispatches a prepared request to a registered local handler.
func (t *LocalTransport) Call(ctx context.Context, req *PreparedRequest) (*RawResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("local transport: nil request")
	}
	t.mu.RLock()
	h, ok := t.routes[routeKey(req.Service, req.Operation)]
	fb := t.fallback
	t.mu.RUnlock()
	if ok {
		return h(ctx, req)
	}
	if fb != nil {
		return fb(ctx, req)
	}
	return nil, &noRouteError{service: req.Service, operation: req.Operation}
}

func routeKey(service, operation string) string {
	var b strings.Builder
	b.WriteString(service)
	b.WriteByte('.')
	b.WriteString(operation)
	return b.String()
}

// noRouteError is a foreign error; the client normalizes it to
// KindRuntime so its concrete type never leaks to callers.
type noRouteError struct {
	service   string
	operation string
}

func (e *noRouteError) Error() string {
	return fmt.Sprintf("local backend 404-style miss: %s.%s is not deployed",
		e.service, e.operation)
}

// FailingTransport is a test/demo transport that always fails with a
// foreign error type. It proves backend error types are normalized.
type FailingTransport struct {
	Foreign string
}

// NewFailingTransport builds a FailingTransport with the given label.
func NewFailingTransport(label string) *FailingTransport {
	return &FailingTransport{Foreign: label}
}

// Call always returns a vendor-specific error.
func (t *FailingTransport) Call(_ context.Context, _ *PreparedRequest) (*RawResponse, error) {
	return nil, &vendorPanicError{label: t.Foreign}
}

type vendorPanicError struct{ label string }

func (e *vendorPanicError) Error() string {
	return "vendor-sdk<" + e.label + ">: exploded in native layer (code 0xDEAD)"
}
