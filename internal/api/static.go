package api

import (
	"net/http"

	"github.com/karthangar/matteresp32hub/web"
)

// staticHandler returns an http.Handler serving the embedded Svelte build.
// Paths not matching a file fall back to index.html for SPA routing.
func staticHandler() http.Handler {
	dist := web.DistFS()
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[1:] // strip leading /
		if path == "" {
			path = "."
		}
		f, err := dist.Open(path)
		if err == nil {
			stat, statErr := f.Stat()
			f.Close()
			if statErr == nil && !stat.IsDir() {
				// Real file — serve it directly
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// No file or it's a directory — fall back to index.html for SPA routing
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
