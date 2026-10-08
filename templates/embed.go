// Package builtin provides the templates shipped with the gateway.
package builtin

import (
	"embed"
	"io/fs"
)

//go:embed html text
var files embed.FS

// FS exposes only template files, excluding Go source files.
func FS() fs.FS { return files }
