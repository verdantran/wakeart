// Package wakeart carries the default scene deck compiled into the binary.
package wakeart

import (
	"embed"
	"io/fs"
)

//go:embed scenes
var scenesFS embed.FS

// Scenes returns the embedded deck rooted at the scenes directory, so entries
// read as "cube.scene".
func Scenes() fs.FS {
	sub, err := fs.Sub(scenesFS, "scenes")
	if err != nil {
		return scenesFS
	}
	return sub
}
