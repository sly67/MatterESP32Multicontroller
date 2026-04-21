package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
)

func jobsRouter(queue *esphome.Queue, database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/", createJob(queue))
		r.Get("/", listJobs(database))
		r.Get("/{id}", getJob(database))
		r.Get("/{id}/stream", streamJob(queue))
		r.Delete("/{id}", cancelJob(queue))
		r.Post("/{id}/resubmit", resubmitJob(queue, database))
		r.Get("/{id}/firmware", serveFirmware(database, queue.DataDir()))
		r.Get("/{id}/manifest.json", serveJobManifest(database))
	}
}

func createJob(queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Board         string                    `json:"board"`
			DeviceName    string                    `json:"device_name"`
			WiFiSSID      string                    `json:"wifi_ssid"`
			WiFiPassword  string                    `json:"wifi_password"`
			HAIntegration bool                      `json:"ha_integration"`
			Components    []esphome.ComponentConfig `json:"components"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.DeviceName == "" || req.Board == "" {
			http.Error(w, "device_name and board are required", http.StatusBadRequest)
			return
		}

		deviceID, err := randomHex(6)
		if err != nil {
			http.Error(w, "generate device id: "+err.Error(), http.StatusInternalServerError)
			return
		}
		otaBuf := make([]byte, 16)
		rand.Read(otaBuf) //nolint:errcheck
		otaPassword := hex.EncodeToString(otaBuf)

		var apiKey string
		if req.HAIntegration {
			keyBuf := make([]byte, 32)
			rand.Read(keyBuf) //nolint:errcheck
			apiKey = base64.StdEncoding.EncodeToString(keyBuf)
		}

		id, err := queue.Enqueue(esphome.JobConfig{
			Board:         req.Board,
			DeviceName:    req.DeviceName,
			DeviceID:      deviceID,
			WiFiSSID:      req.WiFiSSID,
			WiFiPassword:  req.WiFiPassword,
			HAIntegration: req.HAIntegration,
			APIKey:        apiKey,
			OTAPassword:   otaPassword,
			Components:    req.Components,
		})
		if err != nil {
			http.Error(w, "enqueue: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": id}) //nolint:errcheck
	}
}

func listJobs(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := database.ListJobs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if jobs == nil {
			jobs = []db.ESPHomeJob{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobs) //nolint:errcheck
	}
}

func getJob(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := database.GetJob(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job) //nolint:errcheck
	}
}

func streamJob(queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ch, cleanup, err := queue.Subscribe(id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer cleanup()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, _ := w.(http.Flusher)

		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
				if ev.Type == "done" {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

func cancelJob(queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := queue.Cancel(id); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func resubmitJob(queue *esphome.Queue, database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := database.GetJob(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var cfg esphome.JobConfig
		if err := json.Unmarshal([]byte(job.ConfigJSON), &cfg); err != nil {
			http.Error(w, "corrupt config_json: "+err.Error(), http.StatusInternalServerError)
			return
		}
		newID, err := queue.Enqueue(cfg)
		if err != nil {
			http.Error(w, "enqueue: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": newID}) //nolint:errcheck
	}
}

func serveFirmware(database *db.Database, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := database.GetJob(id)
		if err != nil || job.BinaryPath == "" {
			http.Error(w, "firmware not available", http.StatusNotFound)
			return
		}
		cleanPath := filepath.Clean(job.BinaryPath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(dataDir)+string(os.PathSeparator)) {
			http.Error(w, "firmware not available", http.StatusNotFound)
			return
		}
		f, err := os.Open(cleanPath)
		if err != nil {
			http.Error(w, "firmware not available", http.StatusNotFound)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="firmware-factory.bin"`)
		io.Copy(w, f) //nolint:errcheck
	}
}

func serveJobManifest(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := database.GetJob(id)
		if err != nil || job.BinaryPath == "" || job.Status != "done" {
			http.Error(w, "firmware not available", http.StatusNotFound)
			return
		}
		var cfg esphome.JobConfig
		json.Unmarshal([]byte(job.ConfigJSON), &cfg) //nolint:errcheck
		chipFamily := boardToChipFamilyJob(cfg.Board)
		manifest := map[string]interface{}{
			"name":    job.DeviceName,
			"version": id[:6],
			"builds": []map[string]interface{}{
				{
					"chipFamily": chipFamily,
					"parts": []map[string]interface{}{
						{"path": fmt.Sprintf("/api/jobs/%s/firmware", id), "offset": 0},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest) //nolint:errcheck
	}
}

func boardToChipFamilyJob(board string) string {
	switch board {
	case "esp32-c3", "esp32-c3-devkitm-1":
		return "ESP32-C3"
	case "esp32-s3":
		return "ESP32-S3"
	case "esp32-s2":
		return "ESP32-S2"
	case "esp32-h2":
		return "ESP32-H2"
	default:
		return "ESP32-C3"
	}
}
