package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/matter"
)

func devicesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listDevices(database))
		r.Get("/{id}/pairing", getPairingInfo(database))
		r.Get("/{id}/esphome-key", getESPHomeKey(database))
		r.Post("/{id}/heartbeat", heartbeat(database))
		r.Get("/{id}", getDevice(database))
		r.Delete("/{id}", deleteDevice(database))
	}
}

type deviceListItem struct {
	db.Device
	ESPHomeBoard string `json:"esphome_board,omitempty"`
}

func listDevices(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		devs, err := database.ListDevices()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]deviceListItem, 0, len(devs))
		for _, d := range devs {
			item := deviceListItem{Device: d}
			if d.FirmwareType == "esphome" && d.ESPHomeConfig != "" {
				var cfg struct {
					Board string `json:"board"`
				}
				if json.Unmarshal([]byte(d.ESPHomeConfig), &cfg) == nil {
					item.ESPHomeBoard = cfg.Board
				}
			}
			items = append(items, item)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
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
		if dev.MatterPasscode == 0 {
			http.Error(w, "pairing credentials not available for this device", http.StatusNotFound)
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

func heartbeat(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dev, err := database.GetDevice(id)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && dev.FirmwareType != "esphome") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ip := r.RemoteAddr
		if i := strings.LastIndex(ip, ":"); i >= 0 {
			ip = ip[:i]
		}
		if err := database.UpdateDeviceStatus(id, "online", ip); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteDevice(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := database.DeleteDevice(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getESPHomeKey(database *db.Database) http.HandlerFunc {
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
		if dev.ESPHomeAPIKey == "" {
			http.Error(w, "no HA API key for this device", http.StatusNotFound)
			return
		}
		var cfg struct {
			OTAPassword string `json:"ota_password"`
		}
		json.Unmarshal([]byte(dev.ESPHomeConfig), &cfg) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"api_key":      dev.ESPHomeAPIKey,
			"ota_password": cfg.OTAPassword,
		})
	}
}
