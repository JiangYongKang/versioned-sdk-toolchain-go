package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
)

// Artifact is one generated file with its relative path and content hash.
type Artifact struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

// ServiceRecord captures one accepted service version in a generation.
type ServiceRecord struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Codec    string `json:"codec"`
	Source   string `json:"source"`
	InputSHA string `json:"input_sha256"`
	NumOps   int    `json:"operations"`
}

// Manifest is the published, machine-readable result of one generation.
type Manifest struct {
	Version   int             `json:"manifest_version"`
	InputSHA  string          `json:"input_sha256"`
	Artifacts []Artifact      `json:"artifacts"`
	Services  []ServiceRecord `json:"services"`
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashFiles(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(files[n])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func manifestPath(root string) string {
	return filepath.Join(root, sdkDirName, manifestName)
}

func historyPath(root string) string {
	return filepath.Join(root, sdkDirName, historyName)
}

func loadManifest(root string) (*Manifest, error) {
	b, err := os.ReadFile(manifestPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m Manifest
	if err := decodeJSON(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func saveManifest(root string, m *Manifest) ([]byte, error) {
	b, err := stableJSON(m)
	if err != nil {
		return nil, err
	}
	return b, os.WriteFile(manifestPath(root), b, 0o644)
}
