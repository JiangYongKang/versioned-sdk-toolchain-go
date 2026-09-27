package codegen

import (
	"fmt"
	"go/format"
)

// formatGo normalizes rendered Go source with the standard formatter so the
// emitted SDK is always gofmt-clean. Invalid generated source is a generator
// bug and is reported, never silently written out.
func formatGo(path string, src []byte) ([]byte, error) {
	out, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("generated file %s is not valid Go: %w", path, err)
	}
	return out, nil
}
