package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/matter"
)

func devicesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listDevices(database))
		r.Get("/{id}/pairing", getPairingInfo(database))
		r.Get("/{id}", getDevice(database))
	}
}

func listDevices(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		devs, err := database.ListDevices()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if devs == nil {
			devs = []db.Device{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(devs)
	}
}

func getPairingInfo(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dev, err := database.GetDevice(id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"discriminator": dev.MatterDiscrim,
			"passcode":      dev.MatterPasscode,
			"qr_payload":    matter.SetupQRPayload(dev.MatterDiscrim, dev.MatterPasscode),
		})
	}
}

func getDevice(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dev, err := database.GetDevice(id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dev)
	}
}
