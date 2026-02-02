package embed

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var FS embed.FS

// Get content from embedded `static`
func Content() (fs.FS, error) {
	return fs.Sub(FS, "static")
}
