// Package web embeds the frontend SPA assets for serving via the API.
package web

import "embed"

// DistFS contains the embedded frontend SPA build artifacts.
//
//go:embed all:dist/*
var DistFS embed.FS
