package web

import "embed"

// Files contains the embedded single-page application and PWA assets.
//go:embed index.html assets fonts icons manifest.webmanifest sw.js
var Files embed.FS
