package ota

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
)

type checkResponse struct {
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
}

// NewMux returns an http.Handler for the OTA endpoints.
// firmwareDir is the directory where firmware .bin files are stored (e.g. /data/firmware).
func NewMux(database *db.Database, firmwareDir string) http.Handler {
	r := chi.NewRouter()
	r.Use(authMiddleware(database))
	r.Get("/ota/check", handleCheck(database))
	r.Get("/ota/download", handleDownload(database, firmwareDir))
	return r
}

func authMiddleware(database *db.Database) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := authenticate(r, database); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func handleCheck(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dev, err := authenticate(r, database)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		reportedVer := r.Header.Get("X-FW-Version")
		ip := r.RemoteAddr

		if err := database.UpdateDeviceFWVersion(dev.ID, reportedVer, ip); err != nil {
			log.Printf("ota check: update device %s: %v", dev.ID, err)
		}

		latest, err := database.GetLatestFirmware()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(checkResponse{LatestVersion: "", UpdateAvailable: false})
			return
		}

		resp := checkResponse{
			LatestVersion:   latest.Version,
			UpdateAvailable: reportedVer != latest.Version && latest.Version != "",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func handleDownload(database *db.Database, firmwareDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dev, err := authenticate(r, database)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		latest, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusNotFound)
			return
		}

		binPath := filepath.Join(firmwareDir, fmt.Sprintf("%s.bin", latest.Version))
		f, err := os.Open(binPath)
		if err != nil {
			http.Error(w, "firmware file not found", http.StatusNotFound)
			return
		}
		defer f.Close()

		_ = database.CreateOTALog(db.OTALogRow{
			DeviceID: dev.ID,
			FromVer:  dev.FWVersion,
			ToVer:    latest.Version,
			Result:   "ok",
		})

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.bin"`, latest.Version))
		io.Copy(w, f)
	}
}
