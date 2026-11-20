//go:build production

package main

// productionBuild is false in `just dev-wails` and tests, where replacing the
// running binary would replace a Go build cache file.
const productionBuild = true
