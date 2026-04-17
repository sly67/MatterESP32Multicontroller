package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
)

func firmwareRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listFirmware(database))
		r.Post("/", uploadFirmware(database))
		r.Get("/{version}", getFirmware(database))
		r.Post("/{version}/set-latest", setLatestFirmware(database))
		r.Delete("/{version}", deleteFirmware(database))
	}
}

func listFirmware(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fws, err := database.ListFirmware()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if fws == nil {
			fws = []db.FirmwareRow{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fws)
	}
}

func uploadFirmware(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, "multipart parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		version := strings.TrimSpace(r.FormValue("version"))
		boards := strings.TrimSpace(r.FormValue("boards"))
		notes := strings.TrimSpace(r.FormValue("notes"))
		if version == "" || boards == "" {
			http.Error(w, "version and boards are required", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file field missing: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		fwDir := filepath.Join(dataDir, "firmware")
		if err := os.MkdirAll(fwDir, 0755); err != nil {
			http.Error(w, "mkdir: "+err.Error(), http.StatusInternalServerError)
			return
		}
		destPath := filepath.Join(fwDir, version+".bin")
		dest, err := os.Create(destPath)
		if err != nil {
			http.Error(w, "create file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer dest.Close()
		if _, err := io.Copy(dest, file); err != nil {
			http.Error(w, "write file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := database.CreateFirmware(db.FirmwareRow{
			Version: version, Boards: boards, Notes: notes,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func getFirmware(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		version := chi.URLParam(r, "version")
		fw, err := database.GetFirmware(version)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fw)
	}
}

func setLatestFirmware(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		version := chi.URLParam(r, "version")
		if err := database.SetLatestFirmware(version); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteFirmware(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		version := chi.URLParam(r, "version")
		if err := database.DeleteFirmware(version); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		os.Remove(filepath.Join(dataDir, "firmware", version+".bin"))
		w.WriteHeader(http.StatusNoContent)
	}
}
