// Package tmuxmodal holds the bundled application specs.
package tmuxmodal

import "embed"

// Specs is the bundled spec directory (specs/*.toml, specs/groups/*.toml).
//
//go:embed specs
var Specs embed.FS
