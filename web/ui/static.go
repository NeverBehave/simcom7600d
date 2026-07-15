//go:build !headless

// Package uistatic embeds the compiled SPA in web/ui/dist and exposes an
// http.Handler that serves it with SPA history-mode fallback.
//
// At build time, //go:embed picks up the production bundle in web/ui/dist.
// The bundle is committed so ordinary Go and Nix builds always serve a
// complete UI; run `make ui-build` whenever the React sources change.
package uistatic

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var distFS embed.FS

// Handler returns an http.Handler that serves files from dist/ and falls back
// to dist/index.html for any path that does not look like a static asset
// (i.e., contains no file extension). This makes react-router history mode
// work for deep links like /sms/abc123 served directly by the Go binary.
//
// Cache headers:
//   - /assets/* (hashed bundles): immutable, 1 year
//   - everything else (incl. index.html): no-cache
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("uistatic: dist sub-FS: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "" {
			p = "/"
		}
		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		// SPA fallback: if no file extension and not root, rewrite to /.
		if p != "/" && path.Ext(p) == "" {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
