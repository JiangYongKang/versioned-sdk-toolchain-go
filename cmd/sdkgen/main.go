// Command sdkgen is the local, reusable schema-driven SDK generation
// toolchain CLI. It never contacts external services and needs no
// credentials.
//
// Usage:
//
//	sdkgen generate --in ./testdata/schemas --out ./.output --module example.com/mysdk
//	sdkgen validate --in ./testdata/schemas
//
// Exit codes: 0 success; 2 invalid arguments/input; 3 version
// incompatibility; 4 generation failure.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/compat"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/generator"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/loader"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/schema"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/serde"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sdkgen:", err)
		switch {
		case sdkerrors.IsKind(err, sdkerrors.KindInput):
			os.Exit(2)
		case sdkerrors.IsKind(err, sdkerrors.KindVersion):
			os.Exit(3)
		default:
			os.Exit(4)
		}
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("missing subcommand")
	}
	switch args[0] {
	case "generate":
		return cmdGenerate(context.Background(), args[1:])
	case "validate":
		return cmdValidate(context.Background(), args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `sdkgen - local schema-driven SDK toolchain

Commands:
  generate   Load schemas, gate compatibility, emit SDK + metadata
  validate   Load schemas and print compatibility reports without writing

Flags (generate):
  --in DIR       input directory of interface descriptions
  --out DIR      output root for generated SDK and state
  --module PATH  generated Go module path (default sdk-generated)
  --codec NAME   register the built-in "textkv" example codec too
`)
}

func cmdGenerate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	in := fs.String("in", "", "input directory")
	out := fs.String("out", "", "output root")
	mod := fs.String("module", "sdk-generated", "generated Go module path")
	extraCodec := fs.String("codec", "", "extra codec to register (textkv)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" {
		return sdkerrors.New(sdkerrors.KindInput, "generate requires --in and --out")
	}
	registry := serde.MustDefaultRegistry()
	if *extraCodec == serde.TextKVName {
		if err := registry.Register(serde.TextKVCodec{}); err != nil {
			return err
		}
	} else if *extraCodec != "" {
		return sdkerrors.Newf(sdkerrors.KindInput, "unknown built-in codec %q", *extraCodec)
	}
	g := generator.New(generator.Config{
		OutputRoot: *out,
		ModulePath: *mod,
		Serde:      registry,
		Loader:     loader.New(loader.Options{}),
	})
	res, err := g.Generate(ctx, *in)
	if err != nil {
		return err
	}
	fmt.Printf("generate: reused=%t artifacts=%d\n", res.Reused, len(res.Artifacts))
	if res.Reused {
		fmt.Println("generate: input unchanged; previous result reused (deterministic, no rewrite)")
	} else {
		for _, r := range res.Reports {
			printReport(r)
		}
		fmt.Println("manifest:", res.Manifest)
	}
	return nil
}

func cmdValidate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	in := fs.String("in", "", "input directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" {
		return sdkerrors.New(sdkerrors.KindInput, "validate requires --in")
	}
	l := loader.New(loader.Options{})
	doc, err := l.LoadDir(ctx, *in)
	if err != nil {
		return err
	}
	fmt.Printf("validate: loaded %d interface description(s)\n", len(doc.Bundles))
	services := map[string][]*schema.Bundle{}
	for _, b := range doc.Bundles {
		services[b.Interface.Name] = append(services[b.Interface.Name], b)
	}
	var foundBreaking bool
	for name, bundles := range services {
		sort.Slice(bundles, func(i, j int) bool {
			vi, _ := schema.ParseVersion(bundles[i].Interface.Version)
			vj, _ := schema.ParseVersion(bundles[j].Interface.Version)
			return vi.Compare(vj) < 0
		})
		for i := 1; i < len(bundles); i++ {
			rep, err := compat.Compare(bundles[i-1].Interface, bundles[i].Interface)
			if err != nil {
				return sdkerrors.New(sdkerrors.KindGeneration,
					"comparison failed").WithService(name).Wrap(err)
			}
			printReport(rep)
			if !rep.Compatible() {
				foundBreaking = true
			}
		}
	}
	if foundBreaking {
		return sdkerrors.New(sdkerrors.KindVersion,
			"validation found breaking changes (see reports above)")
	}
	fmt.Println("validate: OK")
	return nil
}

func printReport(r *compat.Report) {
	verdict := "COMPATIBLE"
	if !r.Compatible() {
		verdict = "BREAKING"
	}
	fmt.Printf("- %s %s -> %s: %s (%d change(s))\n",
		r.Service, r.FromVer, r.ToVer, verdict, len(r.Changes))
	for _, c := range r.Changes {
		marker := " "
		if c.Kind == compat.ChangeBreaking {
			marker = "!"
		}
		where := c.Field
		if c.Op != "" {
			where = c.Op + "/" + where
		}
		fmt.Printf("   %s %-10s %-40s %s\n", marker, c.Kind, where, c.Detail)
	}
}
