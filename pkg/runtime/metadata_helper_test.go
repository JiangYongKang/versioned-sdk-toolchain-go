package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/codegen"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

// buildMetadataExported produces metadata via the real generator and returns
// the exact metadata.json bytes, exercising the full metadata round-trip.
func buildMetadataExported(t *testing.T, b *schema.Bundle) []byte {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sdk")
	res, err := codegen.Generate(b, codegen.Options{OutputDir: dir, Package: "sdk"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(res.Metadata) {
		t.Fatal("metadata.json on disk differs from Result.Metadata")
	}
	return data
}
