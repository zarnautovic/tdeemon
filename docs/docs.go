// Package docs holds the pages of the documentation the binary serves as
// they are written here, so they cannot drift from it.
package docs

import _ "embed"

// Methodology is methodology.md: how tdeemon's numbers come about, for a
// coach who reads them.
//
//go:embed methodology.md
var Methodology string
