package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
)

// NewRouter builds and returns the chi HTTP router.
func NewRouter(cfg *config.Config, database *db.Database) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/api/health", handleHealth)
	r.Route("/api/devices", devicesRouter(database))
	r.Route("/api/templates", templatesRouter(database))
	r.Route("/api/modules", modulesRouter(database))
	r.Route("/api/effects", effectsRouter(database))
	r.Route("/api/firmware", firmwareRouter(database))
	r.Route("/api/settings", settingsRouter(cfg, database))

	// Frontend — served from embedded FS (wired in Task 7)
	r.Handle("/*", http.NotFoundHandler())

	return r
}
