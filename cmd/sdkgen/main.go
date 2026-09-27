// Command sdkgen is the local, reusable schema-driven SDK generation
// toolchain CLI. It never contacts external services.
//
// Usage:
//
//	sdkgen generate --in ./testdata/schemas --out ./.output --module example.com/sdk
//	sdkgen validate --in ./testdata/schemas
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sdkgen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	_ = context.Background()
	return nil
}
