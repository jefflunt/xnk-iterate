package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:assets
var embeddedAssets embed.FS

// AssetsFS returns the embedded XNK-styled frontend filesystem rooted at assets/.
func AssetsFS() fs.FS {
	sub, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
