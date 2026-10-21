// Package web embeds the page template and static assets into the binary, so
// the image is a single file and the working directory does not matter.
package web

import "embed"

//go:embed templates static
var FS embed.FS
