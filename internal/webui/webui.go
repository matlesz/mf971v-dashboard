// Package webui serves the embedded single-page dashboard.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var embedded embed.FS

// Handler returns the dashboard file server. The assets are compiled into the
// binary, so no file has to be written to the router.
func Handler() http.Handler {
	sub, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err) // impossible: the embed pattern is fixed at build time
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'")
		files.ServeHTTP(w, r)
	})
}
