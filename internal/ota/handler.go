package ota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	r.Use(middleware.Recoverer)
	r.Use(authMiddleware(database))
	r.Get("/ota/check", handleCheck(database))
	r.Get("/ota/download", handleDownload(database, firmwareDir))
	return r
}

func authMiddleware(database *db.Database) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dev, err := authenticate(r, database)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), contextKey{}, dev)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func handleCheck(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dev := deviceFromContext(r)

		reportedVer := r.Header.Get("X-FW-Version")
		ip := r.RemoteAddr

		if err := database.UpdateDeviceFWVersion(dev.ID, reportedVer, ip); err != nil {
			log.Printf("ota check: update device %s: %v", dev.ID, err)
		}

		latest, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusServiceUnavailable)
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
		dev := deviceFromContext(r)

		latest, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusNotFound)
			return
		}

		if strings.ContainsAny(latest.Version, `/\`) {
			http.Error(w, "invalid firmware version", http.StatusInternalServerError)
			return
		}

		binPath := filepath.Join(firmwareDir, fmt.Sprintf("%s.bin", latest.Version))
		f, err := os.Open(binPath)
		if err != nil {
			http.Error(w, "firmware file not found", http.StatusNotFound)
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.bin"`, latest.Version))
		_, copyErr := io.Copy(w, f)

		result := "ok"
		if copyErr != nil {
			log.Printf("ota download: stream device=%s: %v", dev.ID, copyErr)
			result = "error"
		}
		if err := database.CreateOTALog(db.OTALogRow{
			DeviceID: dev.ID,
			FromVer:  dev.FWVersion,
			ToVer:    latest.Version,
			Result:   result,
		}); err != nil {
			log.Printf("ota download: log entry: %v", err)
		}
	}
}
