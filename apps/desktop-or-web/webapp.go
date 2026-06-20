package webapp

import "embed"

//go:embed static/*
var files embed.FS

func Files() embed.FS {
	return files
}
