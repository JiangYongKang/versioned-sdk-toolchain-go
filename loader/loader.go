// Package loader reads local interface description files into schema bundles.
//
// It performs single-file syntactic decoding and structural validation:
// missing references, type collisions, cycles and illegal versions are
// reported with service / field / reason detail rather than skipped.
package loader

import (
	"context"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
)

// Options controls directory loading.
type Options struct {
	// Extensions are accepted file extensions (e.g. ".json").
	Extensions []string
}

// Loader loads interface descriptions from the local filesystem.
type Loader struct {
	opts Options
}

// New creates a Loader with the given options.
func New(opts Options) *Loader { return nil }

// LoadDir loads every description file under dir (non-recursive per
// service/version subdirectory layout) and returns a validated document.
func (l *Loader) LoadDir(ctx context.Context, dir string) (*schema.Document, error) {
	return nil, nil
}

// LoadFile loads and validates a single description file.
func (l *Loader) LoadFile(ctx context.Context, path string) (*schema.Bundle, error) {
	return nil, nil
}
