package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
)

func otaRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/log/{deviceID}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "deviceID")
			entries, err := database.ListOTALogForDevice(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if entries == nil {
				entries = []db.OTALogRow{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entries)
		})
	}
}
