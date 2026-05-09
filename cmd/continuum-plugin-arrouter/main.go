package main

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed manifest.json
var manifestRaw []byte

func main() {
	fmt.Fprintln(os.Stderr, "continuum-plugin-arrouter: stub")
	os.Exit(0)
}
