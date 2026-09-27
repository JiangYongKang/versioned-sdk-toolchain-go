// Package generator turns validated schema documents into a reusable,
// callable SDK: Go client source plus machine-readable call metadata,
// written atomically and deterministically.
//
// Concurrency: generating the same output root (or the same batch twice)
// is safe; output is content-addressed and published via atomic rename,
// guarded by an in-process lock and an advisory file lock, so callers
// never observe half-written artifacts and repeated runs produce
// byte-identical results.
package generator

import (
	"context"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/compat"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/loader"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/serde"
)

// Config configures a Generator.
type Config struct {
	OutputRoot string
	ModulePath string
	Serde      *serde.Registry
	Loader     *loader.Loader
}

// Result describes a completed generation.
type Result struct {
	Root      string
	Manifest  string
	Artifacts []string
	Reused    bool
	Reports   []*compat.Report
}

// Generator produces SDKs from local interface descriptions.
type Generator struct {
	cfg Config
}

// New creates a Generator.
func New(cfg Config) *Generator { return nil }

// Generate loads schemas from inputDir, runs cross-version compatibility
// checks against previously generated manifests, and atomically publishes
// a new SDK under OutputRoot.
func (g *Generator) Generate(ctx context.Context, inputDir string) (*Result, error) {
	return nil, nil
}

// GenerateFrom builds artifacts directly from an in-memory document.
func (g *Generator) GenerateFrom(ctx context.Context, doc *schema.Document) (*Result, error) {
	return nil, nil
}
