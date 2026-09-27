package generator

import (
	"path/filepath"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// render produces every output file for the accepted plan.
//
// File paths are deterministic:
//
//	metadata/<service>/<version>.json          call metadata
//	go/go.mod                                  generated module
//	go/sdkerrors/sdkerrors.go                  copied minimal taxonomy
//	go/runtime/{doc,errors,runtime}.go         copied minimal runtime
//	go/sdk/<service>/<version>/<service>.go    typed client
//	go/sdk/<service>/<version>/types.go        named types
//	go/sdk/<service>/<version>/metadata.json   embedded metadata
func (g *Generator) render(plan *buildPlan) (map[string][]byte, []ServiceRecord, error) {
	files := map[string][]byte{}
	var records []ServiceRecord

	for _, sp := range plan.services {
		for _, bun := range sp.bundles {
			iface := bun.Interface

			// Call metadata (language-agnostic).
			mb, err := stableJSON(buildMetadata(iface))
			if err != nil {
				return nil, nil, sdkerrors.New(sdkerrors.KindGeneration,
					"cannot encode call metadata").
					WithService(iface.Name).WithVersion(iface.Version).Wrap(err)
			}
			metaRel := filepath.Join(metadataDir, iface.Name, iface.Version+".json")
			files[metaRel] = mb

			// Go SDK.
			if err := renderGoPackage(g.cfg.ModulePath, iface, mb, files); err != nil {
				return nil, nil, err
			}

			records = append(records, ServiceRecord{
				Name:     iface.Name,
				Version:  iface.Version,
				Codec:    iface.Codec,
				Source:   bun.Source,
				InputSHA: hashBytes(mb),
				NumOps:   len(iface.Operations),
			})
		}
	}

	// Shared scaffolding, identical across runs (single source of truth
	// embedded from template literals).
	for rel, content := range staticGoScaffold(g.cfg.ModulePath) {
		files[filepath.Join(goSDKDir, rel)] = []byte(content)
	}
	return files, records, nil
}
