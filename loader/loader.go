// Package loader reads local interface description files into schema bundles.
//
// It performs single-file decoding and structural validation: missing
// references, type collisions, cycles and illegal versions are reported
// with service / field / reason detail rather than skipped.
//
// Accepted on-disk layout (all local, no external services):
//
//	root/
//	  billing/v1.json            -> service "billing", version "v1"
//	  billing/v2.json
//	  billing/billing@v3.json    -> alternative "{service}@{version}.json" form
//
// Only *.json files are read; a missing or malformed file is an error.
package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// Options controls directory loading.
type Options struct {
	// Extensions are accepted file extensions (default ".json").
	Extensions []string
}

// Loader loads interface descriptions from the local filesystem.
type Loader struct {
	opts Options
}

// fileHint locates one description file and its expected service name.
type fileHint struct {
	path    string
	service string
}

// New creates a Loader with the given options.
func New(opts Options) *Loader {
	if len(opts.Extensions) == 0 {
		opts.Extensions = []string{".json"}
	}
	return &Loader{opts: opts}
}

// LoadDir loads every description file under dir (recursively, one
// service/version per file) and returns a validated document.
func (l *Loader) LoadDir(ctx context.Context, dir string) (*schema.Document, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, sdkerrors.New(sdkerrors.KindInput, "cannot read input directory").
			WithService(filepath.Base(dir)).Wrap(err)
	}

	var files []fileHint
	for _, e := range entries {
		if e.IsDir() {
			subFiles, err := l.readServiceDir(filepath.Join(dir, e.Name()), e.Name())
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
			continue
		}
		if l.accept(e.Name()) {
			svc := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if i := strings.LastIndex(svc, "@"); i >= 0 {
				svc = svc[:i]
			}
			files = append(files, fileHint{filepath.Join(dir, e.Name()), svc})
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	doc := &schema.Document{}
	seen := map[string]string{} // service|canonicalVersion -> path
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := l.LoadFile(ctx, f.path)
		if err != nil {
			return nil, err
		}
		key := b.Interface.Name + "|" + b.Interface.Version
		if first, ok := seen[key]; ok {
			return nil, sdkerrors.New(sdkerrors.KindInput,
				"duplicate interface version: same service and version appears in multiple files").
				WithService(b.Interface.Name).WithVersion(b.Interface.Version).
				Wrap(fmt.Errorf("%s conflicts with %s", f.path, first))
		}
		seen[key] = f.path
		doc.Bundles = append(doc.Bundles, b)
	}
	if len(doc.Bundles) == 0 {
		return nil, sdkerrors.New(sdkerrors.KindInput,
			"no interface description files found").WithService(filepath.Base(dir))
	}
	return doc, nil
}

func (l *Loader) readServiceDir(dir, service string) ([]fileHint, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, sdkerrors.New(sdkerrors.KindInput, "cannot read service directory").
			WithService(service).Wrap(err)
	}
	var out []fileHint
	for _, e := range entries {
		if e.IsDir() {
			return nil, sdkerrors.New(sdkerrors.KindInput,
				"unexpected nested directory; layout is <root>/<service>/<version>.json").
				WithService(service).WithField(e.Name())
		}
		if !l.accept(e.Name()) {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		svc := service
		if i := strings.LastIndex(name, "@"); i >= 0 {
			svc = name[:i]
		}
		out = append(out, fileHint{filepath.Join(dir, e.Name()), svc})
	}
	return out, nil
}

func (l *Loader) accept(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, x := range l.opts.Extensions {
		if ext == strings.ToLower(x) {
			return true
		}
	}
	return false
}

// LoadFile loads and validates a single description file.
func (l *Loader) LoadFile(ctx context.Context, path string) (*schema.Bundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, sdkerrors.New(sdkerrors.KindInput, "cannot read interface description file").Wrap(err)
	}
	var dto fileDTO
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dto); err != nil {
		return nil, sdkerrors.New(sdkerrors.KindInput, "invalid JSON in interface description").
			Wrap(fmt.Errorf("source=%s: %w", path, err))
	}
	if dec.More() {
		return nil, sdkerrors.New(sdkerrors.KindInput,
			"interface description file contains multiple JSON values").
			Wrap(fmt.Errorf("source=%s", path))
	}

	iface, err := convertAndValidate(&dto, path)
	if err != nil {
		return nil, err
	}
	return &schema.Bundle{Interface: iface, Source: path}, nil
}
