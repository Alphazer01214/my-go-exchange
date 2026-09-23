package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticRoot embed.FS

func staticFS() fs.FS {
	sub, err := fs.Sub(staticRoot, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
