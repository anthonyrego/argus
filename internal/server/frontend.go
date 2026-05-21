package server

import (
	"embed"
	"io/fs"
)

// Frontend returns the embedded React build (web/dist), or nil if it hasn't
// been built yet. When nil, the server falls back to a "frontend not built"
// message so the API still works in dev mode.
//
//go:embed all:dist
var distFS embed.FS

func Frontend() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil
	}
	// Probe for index.html. If the dist directory only contains the .gitkeep
	// placeholder, treat it as unbuilt.
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
