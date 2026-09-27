// Package generator turns validated schema documents into a reusable,
// callable SDK: Go client source plus machine-readable call metadata,
// written atomically and deterministically.
//
// Concurrency: generating against the same output root (or the same batch
// twice) is safe. Rendering happens in a private staging directory;
// publication is an atomic rename guarded by an in-process mutex and an
// advisory flock, so callers never observe half-written artifacts. If an
// identical batch is generated again, the previous result is reused
// (Result.Reused) and no files churn.
package generator

import (
	"context"
	"os"
	"path/filepath"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/compat"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/loader"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/serde"
)

// Config configures a Generator.
type Config struct {
	// OutputRoot receives .sdkgen state, metadata and generated Go code.
	OutputRoot string
	// ModulePath names the generated Go module (default "sdk-generated").
	ModulePath string
	// Serde is the codec registry (default: JSON only).
	Serde *serde.Registry
	// Loader is used by Generate (default: loader.New with defaults).
	Loader *loader.Loader
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
func New(cfg Config) *Generator {
	if cfg.ModulePath == "" {
		cfg.ModulePath = defaultModulePath
	}
	if cfg.Serde == nil {
		cfg.Serde = serde.MustDefaultRegistry()
	}
	if cfg.Loader == nil {
		cfg.Loader = loader.New(loader.Options{})
	}
	return &Generator{cfg: cfg}
}

// Generate loads schemas from inputDir, validates them, gates on
// cross-version compatibility and atomically publishes a new SDK.
func (g *Generator) Generate(ctx context.Context, inputDir string) (*Result, error) {
	// Re-derive the loader so codec validation always reflects the
	// configured registry, even when the caller supplied a loader.
	l := loader.New(loader.Options{
		Extensions:  g.cfg.Loader.Extensions(),
		KnownCodecs: g.knownCodecs(),
	})
	doc, err := l.LoadDir(ctx, inputDir)
	if err != nil {
		return nil, err
	}
	return g.GenerateFrom(ctx, doc)
}

// GenerateFrom builds artifacts directly from an in-memory document.
func (g *Generator) GenerateFrom(ctx context.Context, doc *schema.Document) (*Result, error) {
	if doc == nil || len(doc.Bundles) == 0 {
		return nil, sdkerrors.New(sdkerrors.KindInput,
			"nothing to generate: empty document")
	}
	if g.cfg.OutputRoot == "" {
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"output root is not configured")
	}

	mu := rootMutex(g.cfg.OutputRoot)
	mu.Lock()
	defer mu.Unlock()

	flock, err := acquireFileLock(g.cfg.OutputRoot)
	if err != nil {
		return nil, err
	}
	defer flock.release()

	// Fast path: identical input was already generated and published.
	inputSHA := hashDocument(doc)
	prev, err := loadManifest(g.cfg.OutputRoot)
	if err != nil {
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"cannot read previous manifest").Wrap(err)
	}
	if prev != nil && prev.InputSHA == inputSHA {
		return &Result{
			Root:      g.cfg.OutputRoot,
			Manifest:  manifestPath(g.cfg.OutputRoot),
			Artifacts: artifactNames(prev),
			Reused:    true,
		}, nil
	}

	hist, err := loadHistory(g.cfg.OutputRoot)
	if err != nil {
		return nil, err
	}
	plan, err := g.plan(ctx, doc, hist)
	if err != nil {
		return nil, err
	}

	// Render everything into a private staging directory first.
	staging, err := os.MkdirTemp(filepath.Join(g.cfg.OutputRoot), stagesDir+"-*")
	if err != nil {
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"cannot create staging directory").Wrap(err)
	}
	cleanup := func() { _ = os.RemoveAll(staging) }

	files, records, err := g.render(plan)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := writeStaging(staging, files); err != nil {
		cleanup()
		return nil, err
	}
	manifest := &Manifest{
		Version:   1,
		InputSHA:  inputSHA,
		Artifacts: artifactRecords(files),
		Services:  records,
	}
	mb, err := stableJSON(manifest)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(staging, sdkDirName, manifestName), mb, 0o644); err != nil {
		cleanup()
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"cannot stage manifest").Wrap(err)
	}

	// Publish: merge staged artifacts over the live root atomically per
	// file, then persist history and the manifest last.
	if err := g.publish(staging, files, hist, plan, mb); err != nil {
		cleanup()
		return nil, err
	}
	cleanup()

	return &Result{
		Root:      g.cfg.OutputRoot,
		Manifest:  manifestPath(g.cfg.OutputRoot),
		Artifacts: artifactNames(manifest),
		Reused:    false,
		Reports:   plan.reports,
	}, nil
}

func artifactNames(m *Manifest) []string {
	out := make([]string, 0, len(m.Artifacts))
	for _, a := range m.Artifacts {
		out = append(out, a.Path)
	}
	return out
}

func artifactRecords(files map[string][]byte) []Artifact {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sortStrings(names)
	out := make([]Artifact, 0, len(names))
	for _, n := range names {
		out = append(out, Artifact{Path: n, SHA: hashBytes(files[n])})
	}
	return out
}

// hashDocument produces a stable fingerprint of the whole batch,
// independent of map/load ordering, via its stable JSON encoding.
func hashDocument(doc *schema.Document) string {
	type bundleDTO struct {
		Source string            `json:"source"`
		If     *schema.Interface `json:"interface"`
	}
	dto := struct {
		Bundles []bundleDTO `json:"bundles"`
	}{}
	// Copy then sort bundles by service/version for stability.
	bundles := append([]*schema.Bundle(nil), doc.Bundles...)
	sortBundles(bundles)
	for _, b := range bundles {
		dto.Bundles = append(dto.Bundles, bundleDTO{Source: b.Source, If: b.Interface})
	}
	b, err := stableJSON(dto)
	if err != nil {
		// schema types are JSON-encodable by construction.
		panic(sdkerrors.New(sdkerrors.KindGeneration,
			"internal: cannot fingerprint document").Wrap(err))
	}
	return hashBytes(b)
}
