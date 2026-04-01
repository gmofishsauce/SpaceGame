// Package webui exposes the compiled SPA assets for embedding into the server binary.
package webui

import "embed"

//go:embed dist
var DistFS embed.FS
