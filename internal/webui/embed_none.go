//go:build !webui

package webui

import "io/fs"

// embeddedFS reports no web app: this binary was built without the webui
// tag.
func embeddedFS() (fs.FS, error) {
	return nil, nil
}
