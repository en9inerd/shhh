package ui

import "embed"

// Files is embedded static files
//
//go:embed "templates/*" "static/*"
var Files embed.FS
