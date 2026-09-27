package codegen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// dirLocks serializes generation into the same output directory so concurrent
// callers cannot observe each other's half-written artifacts.
var dirLocks = &locker{m: map[string]*sync.Mutex{}}

type locker struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func (l *locker) get(dir string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.m[dir]
	if !ok {
		m = &sync.Mutex{}
		l.m[dir] = m
	}
	return m
}

// artifact is one generated file.
type artifact struct {
	relPath string
	content []byte
}

// writeAtomic stages all artifacts in a sibling temporary directory and then
// swaps it into place with an atomic rename. On failure the staging area is
// removed and the destination is left untouched.
func writeAtomic(outputDir string, arts []artifact) (created []string, err error) {
	mu := dirLocks.get(filepath.Clean(outputDir))
	mu.Lock()
	defer mu.Unlock()

	sort.Slice(arts, func(i, j int) bool { return arts[i].relPath < arts[j].relPath })

	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(parent, ".sdkgen-*")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()

	names := make([]string, 0, len(arts))
	for _, a := range arts {
		full := filepath.Join(tmp, filepath.Clean(a.relPath))
		// Guard against path traversal in relPath.
		rel, err := filepath.Rel(tmp, full)
		if err != nil || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, &pathError{path: a.relPath}
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, a.content, 0o644); err != nil {
			return nil, err
		}
		names = append(names, filepath.ToSlash(a.relPath))
	}

	// Replace destination atomically: remove old, rename new. Rename over an
	// existing non-empty directory is not portable, so remove-then-rename.
	if _, statErr := os.Lstat(outputDir); statErr == nil {
		if err := os.RemoveAll(outputDir); err != nil {
			return nil, err
		}
	}
	if err := os.Rename(tmp, outputDir); err != nil {
		return nil, err
	}
	cleanup = false
	sort.Strings(names)
	return names, nil
}

type pathError struct{ path string }

func (e *pathError) Error() string { return "artifact path escapes output directory: " + e.path }
