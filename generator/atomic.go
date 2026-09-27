package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// writeStaging writes every rendered file into the private staging tree,
// creating parent directories as needed.
func writeStaging(staging string, files map[string][]byte) error {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, rel := range names {
		full := filepath.Join(staging, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return sdkerrors.New(sdkerrors.KindGeneration,
				"cannot create artifact directory").WithField(rel).Wrap(err)
		}
		if err := os.WriteFile(full, files[rel], 0o644); err != nil {
			return sdkerrors.New(sdkerrors.KindGeneration,
				"cannot write staged artifact").WithField(rel).Wrap(err)
		}
	}
	// Ensure the state directory exists in staging even when empty.
	if err := os.MkdirAll(filepath.Join(staging, sdkDirName), 0o755); err != nil {
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot stage state directory").Wrap(err)
	}
	return nil
}

// publish moves staged artifacts into the live root.
//
// Each artifact is published with a temp-file + os.Rename in the same
// directory (atomic on POSIX). Files replaced during the publish are
// backed up first; any failure rolls the backups back so the live tree
// never ends up half-written. History and the manifest are committed
// last, after every artifact landed.
func (g *Generator) publish(staging string, files map[string][]byte, hist *historyFile, plan *buildPlan, manifestBytes []byte) error {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	type backup struct {
		rel string
		old string // temp path holding previous content; "" if previously absent
	}
	var done []backup
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			b := done[i]
			target := filepath.Join(g.cfg.OutputRoot, b.rel)
			_ = os.Remove(target)
			if b.old != "" {
				_ = os.Rename(b.old, target)
			}
		}
	}

	for _, rel := range names {
		src := filepath.Join(staging, rel)
		dst := filepath.Join(g.cfg.OutputRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			rollback()
			return sdkerrors.New(sdkerrors.KindGeneration,
				"cannot create artifact directory").WithField(rel).Wrap(err)
		}
		b := backup{rel: rel}
		if data, err := os.ReadFile(dst); err == nil {
			tmp, err := os.CreateTemp(filepath.Dir(dst), ".bak-*")
			if err != nil {
				rollback()
				return sdkerrors.New(sdkerrors.KindGeneration,
					"cannot create artifact backup").WithField(rel).Wrap(err)
			}
			tmpName := tmp.Name()
			_ = tmp.Close()
			if err := os.WriteFile(tmpName, data, 0o644); err != nil {
				_ = os.Remove(tmpName)
				rollback()
				return sdkerrors.New(sdkerrors.KindGeneration,
					"cannot write artifact backup").WithField(rel).Wrap(err)
			}
			b.old = tmpName
		}

		if err := atomicCopy(src, dst); err != nil {
			if b.old != "" {
				_ = os.Remove(b.old)
			}
			rollback()
			return err
		}
		if b.old != "" {
			_ = os.Remove(b.old)
		}
		done = append(done, b)
	}

	// Commit accepted versions into history.
	for _, sp := range plan.services {
		for _, bun := range sp.bundles {
			hist.Accepted[historyKey(bun.Interface.Name, bun.Interface.Version)] =
				toHistory(bun.Interface)
		}
	}
	if err := hist.save(g.cfg.OutputRoot); err != nil {
		rollback()
		return err
	}
	if err := os.WriteFile(manifestPath(g.cfg.OutputRoot), manifestBytes, 0o644); err != nil {
		rollback()
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot publish manifest").Wrap(err)
	}
	return nil
}

// atomicCopy writes src into dst via a sibling temp file and rename.
func atomicCopy(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot read staged artifact during publish").Wrap(err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".pub-*")
	if err != nil {
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot create publish temp file").Wrap(err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(in); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot write publish temp file").Wrap(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot close publish temp file").Wrap(err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot set artifact permissions").Wrap(err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return sdkerrors.New(sdkerrors.KindGeneration,
			"cannot atomically publish artifact").
			WithField(relOf(dst)).Wrap(fmt.Errorf("rename: %w", err))
	}
	return nil
}

// relOf is best-effort for diagnostics.
func relOf(path string) string { return filepath.Base(path) }
