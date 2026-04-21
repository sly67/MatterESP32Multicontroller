package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
)

func NewRouter(cfg *config.Config, database *db.Database, queue *esphome.Queue) http.Handler {
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
	r.Route("/api/build", buildRouter(cfg))
	r.Route("/api/flash", flashRouter(database, queue))
	r.Route("/api/webflash", webflashRouter(cfg, database, queue))
	r.Route("/api/settings", settingsRouter(cfg, database))
	r.Route("/api/ota", otaRouter(database))
	if queue != nil {
		r.Route("/api/jobs", jobsRouter(queue, database))
	}

	r.Handle("/*", staticHandler())
	return r
}
