package main

import (
	"fmt"
	"os"
)

// version is set at build time via -ldflags "-X main.version=<value>".
// GoReleaser injects the release tag; the install-local.sh script injects
// a dev-<unix-timestamp> string so you can confirm a local build is active.
var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "git-clarity: not yet implemented")
	os.Exit(1)
}
