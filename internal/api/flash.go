package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/flash"
	"github.com/karthangar/matteresp32hub/internal/library"
	"github.com/karthangar/matteresp32hub/internal/usb"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

func flashRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/ports", listPorts)
		r.Post("/run", runFlash(database))
		r.Post("/esphome", runESPHomeFlash(database))
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

func runESPHomeFlash(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Port          string                   `json:"port"`
			DeviceName    string                   `json:"device_name"`
			WiFiSSID      string                   `json:"wifi_ssid"`
			WiFiPassword  string                   `json:"wifi_password"`
			HubURL        string                   `json:"hub_url"`
			Board         string                   `json:"board"`
			HAIntegration bool                     `json:"ha_integration"`
			Components    []esphome.ComponentConfig `json:"components"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Port == "" || req.DeviceName == "" || req.Board == "" || req.HubURL == "" {
			http.Error(w, "port, device_name, board, hub_url are required", http.StatusBadRequest)
			return
		}

		mods, err := library.LoadModules()
		if err != nil {
			http.Error(w, "load modules: "+err.Error(), http.StatusInternalServerError)
			return
		}
		modMap := make(map[string]*yamldef.Module, len(mods))
		for _, m := range mods {
			modMap[m.ID] = m
		}

		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		builder, err := esphome.NewBuilder(dataDir+"/esphome-cache", os.Getenv("ESPHOME_CACHE_VOLUME"))
		if err != nil {
			http.Error(w, "builder init: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer builder.Close()

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Transfer-Encoding", "chunked")
		flusher, canFlush := w.(http.Flusher)

		pr, pw := io.Pipe()
		done := make(chan flash.Result, 1)
		go func() {
			result := flash.FlashESPHomeDevice(database, builder, modMap, flash.ESPHomeRequest{
				Ctx:           r.Context(),
				Port:          req.Port,
				DeviceName:    req.DeviceName,
				WiFiSSID:      req.WiFiSSID,
				WiFiPassword:  req.WiFiPassword,
				HubURL:        req.HubURL,
				Board:         req.Board,
				HAIntegration: req.HAIntegration,
				Components:    req.Components,
			}, pw)
			pw.Close()
			done <- result
		}()

		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			json.NewEncoder(w).Encode(map[string]string{"log": line}) //nolint:errcheck
			if canFlush {
				flusher.Flush()
			}
		}

		result := <-done
		if result.Error != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": result.Error.Error()}) //nolint:errcheck
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "device_id": result.DeviceID, "name": result.Name}) //nolint:errcheck
		}
		if canFlush {
			flusher.Flush()
		}
	}
}
