//go:build webui

package webui

import (
	"embed"
	"io/fs"
)

// dist is the Vite build of web/. Building with the webui tag fails with
// "pattern all:dist: no matching files found" until it has been built.
//
//go:embed all:dist
var dist embed.FS

func embeddedFS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
