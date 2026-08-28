package main

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var embeddedFS embed.FS

func getStaticFS() fs.FS {
	sub, err := fs.Sub(embeddedFS, "web/dist")
	if err != nil {
		return nil
	}
	return sub
}
