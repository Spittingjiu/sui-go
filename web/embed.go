package web

import (
	"embed"
	"io/fs"
)

// assets embeds the panel web UI so a standalone binary can serve it
// without requiring a public/ directory next to the working directory.
//
//go:embed all:public
var assets embed.FS

// Public returns the embedded web UI filesystem rooted at public/.
func Public() fs.FS {
	sub, err := fs.Sub(assets, "public")
	if err != nil {
		return assets
	}
	return sub
}
