package main

import (
	"fmt"
	"os"

	"github.com/xchebila/reposcan/cli"
)

// version is overridden at build time via -ldflags "-X main.version=...";
// left as "dev" for a plain `go build`/`go run` with no ldflags.
var version = "dev"

func main() {
	if err := cli.NewRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
