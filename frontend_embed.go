// Package steelpage is the project root and exposes the embedded frontend
// build to the cmd binary. Keeping the //go:embed directive here is the only
// way to reach frontend/dist without using forbidden ".." path segments.
package steelpage

import "embed"

//go:embed all:frontend/dist
var FrontendFS embed.FS
