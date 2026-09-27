package generator

import (
	"context"
	"fmt"
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/compat"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// buildPlan groups the loaded document by service, orders versions and
// produces compatibility reports before any file is rendered.
type buildPlan struct {
	services []*servicePlan
	reports  []*compat.Report
}

type servicePlan struct {
	name    string
	bundles []*schema.Bundle // ascending by parsed version
}

// plan validates batch-internal version ordering and runs compatibility
// checks: adjacent versions within the batch, plus each batch version
// against the closest lower version already present in history.
func (g *Generator) plan(ctx context.Context, doc *schema.Document, hist *historyFile) (*buildPlan, error) {
	byName := map[string][]*schema.Bundle{}
	for _, b := range doc.Bundles {
		byName[b.Interface.Name] = append(byName[b.Interface.Name], b)
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)

	p := &buildPlan{}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bundles := append([]*schema.Bundle(nil), byName[name]...)
		sort.Slice(bundles, func(i, j int) bool {
			vi, _ := schema.ParseVersion(bundles[i].Interface.Version)
			vj, _ := schema.ParseVersion(bundles[j].Interface.Version)
			return vi.Compare(vj) < 0
		})
		sp := &servicePlan{name: name, bundles: bundles}

		// Duplicate raw version strings inside one service.
		for i := 1; i < len(bundles); i++ {
			if bundles[i].Interface.Version == bundles[i-1].Interface.Version {
				return nil, sdkerrors.New(sdkerrors.KindInput,
					"duplicate version within one service in the same batch").
					WithService(name).WithVersion(bundles[i].Interface.Version).
					Wrap(fmt.Errorf("sources: %s, %s",
						bundles[i-1].Source, bundles[i].Source))
			}
		}

		// Adjacent comparisons inside the batch.
		for i := 1; i < len(bundles); i++ {
			rep, err := compat.Compare(bundles[i-1].Interface, bundles[i].Interface)
			if err != nil {
				return nil, sdkerrors.New(sdkerrors.KindGeneration,
					"internal comparison failed").Wrap(err)
			}
			p.reports = append(p.reports, rep)
			if !rep.Compatible() {
				return nil, breakingError(rep)
			}
		}

		// Compare the lowest batch version against the closest lower
		// version accepted previously (cross-run guard).
		if len(bundles) > 0 {
			lowest := bundles[0].Interface
			if prev := closestLower(hist, lowest); prev != nil {
				rep, err := compat.Compare(prev, lowest)
				if err != nil {
					return nil, sdkerrors.New(sdkerrors.KindGeneration,
						"history comparison failed").Wrap(err)
				}
				p.reports = append(p.reports, rep)
				if !rep.Compatible() {
					return nil, breakingError(rep)
				}
			}
		}
		p.services = append(p.services, sp)
	}
	return p, nil
}

// closestLower finds the accepted history version strictly lower than
// target. The closest (highest lower) one is the meaningful baseline.
func closestLower(hist *historyFile, target *schema.Interface) *schema.Interface {
	tv, _ := schema.ParseVersion(target.Version)
	var best *schema.Interface
	var bestV schema.Version
	for key, hi := range hist.Accepted {
		// Defensive: only consider entries of the same service.
		var n, v string
		if k, err := parseHistoryKey(key); err == nil {
			n, v = k.name, k.version
		}
		if n != target.Name {
			continue
		}
		hv, err := schema.ParseVersion(v)
		if err != nil {
			continue
		}
		if hv.Compare(tv) < 0 && (best == nil || hv.Compare(bestV) > 0) {
			iface := fromHistory(hi)
			best, bestV = iface, hv
		}
	}
	return best
}

type historyKeyParts struct{ name, version string }

func parseHistoryKey(key string) (historyKeyParts, error) {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '|' {
			return historyKeyParts{key[:i], key[i+1:]}, nil
		}
	}
	return historyKeyParts{}, fmt.Errorf("bad history key %q", key)
}

// breakingError turns a report containing breaking changes into a single
// KindVersion error whose reason enumerates every affected location.
func breakingError(rep *compat.Report) error {
	bc := rep.BreakingChanges()
	var loc string
	for i, c := range bc {
		if i > 0 {
			loc += "; "
		}
		loc += c.Field
		if c.Op != "" {
			loc += " (op " + c.Op + ")"
		}
		loc += ": " + c.Detail
	}
	return sdkerrors.Newf(sdkerrors.KindVersion,
		"breaking change rejected for %s %s -> %s; affected locations: %s",
		rep.Service, rep.FromVer, rep.ToVer, loc).
		WithService(rep.Service).
		WithVersion(rep.FromVer + "->" + rep.ToVer)
}
