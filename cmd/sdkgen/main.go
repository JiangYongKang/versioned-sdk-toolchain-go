// Command sdkgen is the local schema-driven SDK generator.
//
// Usage:
//
//	sdkgen generate -o ./build/sdk -pkg sdk [description.json ...]
//	sdkgen check-compat -old v1.json -new v2.json
//
// All inputs are local files; no network or credentials are used.
// Exit code 2 marks input/version problems, 3 generation failures,
// 1 unexpected internal errors, 0 success.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/codegen"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/compat"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/load"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = cmdGenerate(os.Args[2:])
	case "check-compat":
		err = cmdCheckCompat(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sdkgen:", err)
		switch apierrors.Code(err) {
		case apierrors.CatInvalidInput, apierrors.CatVersionIncompatible:
			os.Exit(2)
		case apierrors.CatGenerationFailed:
			os.Exit(3)
		default:
			os.Exit(1)
		}
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `sdkgen - local schema-driven SDK generator

Commands:
  generate -o DIR [-pkg NAME] [-format json] FILE...
      Validate the description files and generate a Go SDK plus metadata.
  check-compat -old FILE -new FILE
      Compare two versions. Exit 2 with a report when breaking changes exist.
`)
}

func cmdGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	out := fs.String("o", "", "output directory (required)")
	pkg := fs.String("pkg", "sdk", "generated Go package name")
	format := fs.String("format", "json", "wire format recorded in metadata")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files := fs.Args()
	if len(files) == 0 {
		return apierrors.New(apierrors.CatInvalidInput, "generate requires at least one description file").
			WithOp("cli.generate")
	}
	b, err := load.New().LoadFiles(files...)
	if err != nil {
		return err
	}
	res, err := codegen.Generate(b, codegen.Options{OutputDir: *out, Package: *pkg, Format: *format})
	if err != nil {
		return err
	}
	fmt.Printf("generated SDK for %s@%s in %s\nartifacts:\n",
		b.Documents[0].Service, b.Documents[0].Version, res.OutputDir)
	for _, a := range res.Artifacts {
		fmt.Println("  -", a)
	}
	return nil
}

func cmdCheckCompat(args []string) error {
	fs := flag.NewFlagSet("check-compat", flag.ContinueOnError)
	oldPath := fs.String("old", "", "old version description file (required)")
	newPath := fs.String("new", "", "new version description file (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *oldPath == "" || *newPath == "" {
		return apierrors.New(apierrors.CatInvalidInput, "check-compat requires -old and -new").
			WithOp("cli.check-compat")
	}
	loader := load.New()
	oldB, err := loader.LoadFiles(*oldPath)
	if err != nil {
		return err
	}
	newB, err := loader.LoadFiles(*newPath)
	if err != nil {
		return err
	}
	report, err := compat.Compare(oldB, newB)
	if err != nil {
		return err
	}
	for _, c := range report.Changes {
		marker := "OK"
		if c.Kind == compat.ChangeBreaking {
			marker = "BREAK"
		}
		loc := c.Interface
		if loc == "" {
			loc = c.Field
		} else if c.Field != "" {
			loc += "/" + c.Field
		}
		fmt.Printf("[%s] %s -> %s %s: %s\n", marker, report.From, report.To, loc, c.Reason)
	}
	if !report.Compatible() {
		return apierrors.New(apierrors.CatVersionIncompatible,
			fmt.Sprintf("%d breaking change(s) from %s to %s",
				len(report.Breaking()), report.From, report.To)).
			WithOp("cli.check-compat")
	}
	fmt.Printf("versions %s -> %s compatible (%d compatible change(s))\n",
		report.From, report.To, len(report.Changes))
	return nil
}
