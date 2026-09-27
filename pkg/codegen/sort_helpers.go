package codegen

import (
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

// sortEnums orders enums by name for deterministic output.
func sortEnums(es []schema.Enum) {
	sort.Slice(es, func(i, j int) bool { return es[i].Name < es[j].Name })
}

// sortStructs orders structs by name for deterministic output.
func sortStructs(ss []schema.Struct) {
	sort.Slice(ss, func(i, j int) bool { return ss[i].Name < ss[j].Name })
}
