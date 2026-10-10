// Package builtin provides the static assets shipped with the gateway.
package builtin

import (
	"embed"
	"io/fs"
)

//go:embed auth home icons
var files embed.FS

func FS() fs.FS { return files }
