// Package web embeds the frontend SPA assets for serving via the API.
package web

import "embed"

//go:embed all:dist/*
var DistFS embed.FS
