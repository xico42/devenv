package main

import (
	"os"

	"github.com/xico42/devenv/cmd"
)

var version = "dev"

func main() {
	_ = version // set at build time via -ldflags
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
