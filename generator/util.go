package generator

import (
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
)

func sortStrings(s []string) { sort.Strings(s) }

func sortBundles(b []*schema.Bundle) {
	sort.Slice(b, func(i, j int) bool {
		if b[i].Interface.Name != b[j].Interface.Name {
			return b[i].Interface.Name < b[j].Interface.Name
		}
		vi, _ := schema.ParseVersion(b[i].Interface.Version)
		vj, _ := schema.ParseVersion(b[j].Interface.Version)
		return vi.Compare(vj) < 0
	})
}

func (g *Generator) knownCodecs() map[string]bool {
	m := map[string]bool{}
	for _, n := range g.cfg.Serde.Names() {
		m[n] = true
	}
	return m
}
