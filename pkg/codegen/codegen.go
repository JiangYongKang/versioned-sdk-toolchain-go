// Package codegen turns a validated schema.Bundle into a runnable, reusable
// Go SDK plus call metadata. Generation is deterministic: identical inputs
// always produce byte-identical artifacts, and concurrent generation into
// the same directory is serialized with an atomic directory swap, so callers
// can never observe a half-written SDK.
package codegen

import (
	"path/filepath"
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/validate"
)

// Options controls SDK generation.
type Options struct {
	// OutputDir is the directory the SDK package is written into.
	OutputDir string
	// Package is the generated Go package name (default: "sdk").
	Package string
	// Format is the wire format recorded in metadata (default: "json").
	Format string
}

// Result describes a completed generation.
type Result struct {
	OutputDir string
	// Artifacts are slash-separated paths relative to OutputDir.
	Artifacts []string
	// Metadata is the exact bytes written to metadata.json.
	Metadata []byte
}

// Generate validates the bundle and writes the SDK + metadata into
// opts.OutputDir. Validation failures are invalid_input errors; failures
// during writing are generation_failed errors.
func Generate(b *schema.Bundle, opts Options) (*Result, error) {
	if b == nil || len(b.Documents) == 0 {
		return nil, apierrors.New(apierrors.CatInvalidInput, "cannot generate from empty bundle").
			WithOp("codegen.generate")
	}
	if opts.OutputDir == "" {
		return nil, apierrors.New(apierrors.CatInvalidInput, "output directory is required").
			WithOp("codegen.generate")
	}
	if err := validate.Validate(b); err != nil {
		return nil, err
	}
	pkgName := opts.Package
	if pkgName == "" {
		pkgName = "sdk"
	}
	format := opts.Format
	if format == "" {
		format = "json"
	}

	arts, err := renderGoSDK(b, pkgName)
	if err != nil {
		return nil, apierrors.New(apierrors.CatGenerationFailed, err.Error()).
			WithOp("codegen.generate").WithCause(err)
	}
	relPaths := make([]string, 0, len(arts)+1)
	for _, a := range arts {
		relPaths = append(relPaths, a.relPath)
	}
	relPaths = append(relPaths, "metadata.json")
	sort.Strings(relPaths)
	md := buildMetadata(b, format, relPaths)
	arts = append(arts, artifact{relPath: "metadata.json", content: md})

	names, err := writeAtomic(filepath.Clean(opts.OutputDir), arts)
	if err != nil {
		return nil, apierrors.New(apierrors.CatGenerationFailed,
			"failed to write SDK artifacts: "+err.Error()).
			WithOp("codegen.generate").WithCause(err)
	}
	return &Result{OutputDir: opts.OutputDir, Artifacts: names, Metadata: md}, nil
}
