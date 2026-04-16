package api

import (
	"net/http"

	"github.com/karthangar/matteresp32hub/web"
)

// staticHandler returns an http.Handler serving the embedded Svelte build.
// Paths not matching a file fall back to index.html for SPA routing.
func staticHandler() http.Handler {
	dist := web.DistFS()
	fsys := http.FS(dist)
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the exact path; fall back to index.html for SPA routing
		f, err := dist.Open(r.URL.Path[1:])
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
