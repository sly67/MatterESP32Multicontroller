package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/flash"
	"github.com/karthangar/matteresp32hub/internal/usb"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

func flashRouter(database *db.Database, queue *esphome.Queue) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/ports", listPorts)
		r.Post("/run", runFlash(database))
		r.Post("/esphome", runESPHomeFlash(database, queue))
	}
}

func listPorts(w http.ResponseWriter, r *http.Request) {
	ports, err := usb.ListPorts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ports == nil {
		ports = []usb.Port{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ports)
}

func runFlash(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TemplateID   string   `json:"template_id"`
			DeviceNames  []string `json:"device_names"`
			WiFiSSID     string   `json:"wifi_ssid"`
			WiFiPassword string   `json:"wifi_password"`
			Port         string   `json:"port"`
			FWVersion    string   `json:"fw_version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.TemplateID == "" || len(req.DeviceNames) == 0 || req.Port == "" || req.FWVersion == "" {
			http.Error(w, "template_id, device_names, port, fw_version are required", http.StatusBadRequest)
			return
		}

		tplRow, err := database.GetTemplate(req.TemplateID)
		if err != nil {
			http.Error(w, "template not found: "+err.Error(), http.StatusNotFound)
			return
		}
		tpl, err := yamldef.ParseTemplate([]byte(tplRow.YAMLBody))
		if err != nil {
			http.Error(w, "template parse error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		fwPath := filepath.Join(dataDir, "firmware", req.FWVersion+".bin")
		if _, err := os.Stat(fwPath); err != nil {
			http.Error(w, "firmware file not found: "+req.FWVersion, http.StatusNotFound)
			return
		}

		type flashResult struct {
			Name     string `json:"name"`
			DeviceID string `json:"device_id,omitempty"`
			Error    string `json:"error,omitempty"`
			OK       bool   `json:"ok"`
		}
		var results []flashResult
		for _, name := range req.DeviceNames {
			res := flash.FlashDevice(database, flash.Request{
				Port:         req.Port,
				Template:     tpl,
				DeviceName:   name,
				WiFiSSID:     req.WiFiSSID,
				WiFiPassword: req.WiFiPassword,
				FirmwarePath: fwPath,
				FWVersion:    req.FWVersion,
			})
			fr := flashResult{Name: res.Name, DeviceID: res.DeviceID, OK: res.Error == nil}
			if res.Error != nil {
				fr.Error = res.Error.Error()
			}
			results = append(results, fr)
			if res.Error != nil {
				break
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func runESPHomeFlash(_ *db.Database, _ *esphome.Queue) http.HandlerFunc {
	// TODO(Task 6): wire queue-based ESPHome flash here.
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "ESPHome flash via queue not yet implemented — use /api/jobs", http.StatusNotImplemented)
	}
}
