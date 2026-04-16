// Package web exposes the compiled Svelte frontend as an embedded filesystem.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var staticFiles embed.FS

// DistFS returns a filesystem rooted at the web/dist build output.
func DistFS() fs.FS {
	dist, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		panic("embed: web/dist not found — run 'make web' first")
	}
	return dist
}
