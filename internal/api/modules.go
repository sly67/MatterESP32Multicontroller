package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
)

func modulesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	}
}
