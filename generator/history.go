package generator

import (
	"os"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// historyInterface is the canonical persisted form of an accepted
// interface. It uses the loader's JSON encoding indirectly: we store the
// stable in-memory representation via exported model types.
type historyInterface struct {
	Name       string             `json:"name"`
	Version    string             `json:"version"`
	Codec      string             `json:"codec"`
	Doc        string             `json:"doc,omitempty"`
	Operations []schema.Operation `json:"operations"`
	Types      []schema.NamedType `json:"types"`
}

// historyFile maps "service|version" to its accepted interface record.
// Map keys are serialized via stableJSON (sorted keys).
type historyFile struct {
	Accepted map[string]historyInterface `json:"accepted"`
}

func loadHistory(root string) (*historyFile, error) {
	h := &historyFile{Accepted: map[string]historyInterface{}}
	b, err := os.ReadFile(historyPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"cannot read version history").Wrap(err)
	}
	if err := decodeJSON(b, h); err != nil {
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"version history is corrupted; refusing to overwrite").Wrap(err)
	}
	if h.Accepted == nil {
		h.Accepted = map[string]historyInterface{}
	}
	return h, nil
}

func (h *historyFile) save(root string) error {
	b, err := stableJSON(h)
	if err != nil {
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot serialize version history").Wrap(err)
	}
	if err := os.WriteFile(historyPath(root), b, 0o644); err != nil {
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot write version history").Wrap(err)
	}
	return nil
}

func historyKey(name, version string) string { return name + "|" + version }

func toHistory(iface *schema.Interface) historyInterface {
	return historyInterface{
		Name:       iface.Name,
		Version:    iface.Version,
		Codec:      iface.Codec,
		Doc:        iface.Doc,
		Operations: append([]schema.Operation(nil), iface.Operations...),
		Types:      append([]schema.NamedType(nil), iface.Types...),
	}
}

func fromHistory(h historyInterface) *schema.Interface {
	return &schema.Interface{
		Name:       h.Name,
		Version:    h.Version,
		Codec:      h.Codec,
		Doc:        h.Doc,
		Operations: append([]schema.Operation(nil), h.Operations...),
		Types:      append([]schema.NamedType(nil), h.Types...),
	}
}
