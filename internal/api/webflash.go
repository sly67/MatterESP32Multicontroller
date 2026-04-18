package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	godata "github.com/karthangar/matteresp32hub/data"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
)

func webflashRouter(cfg *config.Config, database *db.Database) func(chi.Router) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	firmwareDir := filepath.Join(dataDir, "firmware")

	return func(r chi.Router) {
		r.Get("/manifest.json", serveWebFlashManifest(database))
		r.Get("/bootloader.bin", serveFlashStatic("flash/esp32c3/bootloader.bin", "bootloader.bin"))
		r.Get("/partition-table.bin", serveFlashStatic("flash/esp32c3/partition-table.bin", "partition-table.bin"))
		r.Get("/ota_data_initial.bin", serveFlashStatic("flash/esp32c3/ota_data_initial.bin", "ota_data_initial.bin"))
		r.Get("/firmware.bin", serveLatestFirmwareBin(database, firmwareDir))
	}
}

func serveWebFlashManifest(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fw, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusServiceUnavailable)
			return
		}
		type part struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
		}
		type build struct {
			ChipFamily string `json:"chipFamily"`
			Parts      []part `json:"parts"`
		}
		manifest := struct {
			Name    string  `json:"name"`
			Version string  `json:"version"`
			Builds  []build `json:"builds"`
		}{
			Name:    "Matter Hub Firmware",
			Version: fw.Version,
			Builds: []build{
				{
					ChipFamily: "ESP32-C3",
					Parts: []part{
						{Path: "/api/webflash/bootloader.bin", Offset: 0x0},
						{Path: "/api/webflash/partition-table.bin", Offset: 0x8000},
						{Path: "/api/webflash/ota_data_initial.bin", Offset: 0xf000},
						{Path: "/api/webflash/firmware.bin", Offset: 0x20000},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}
}

func serveFlashStatic(embedPath, filename string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := godata.FlashFS.ReadFile(embedPath)
		if err != nil {
			http.Error(w, "static file not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
		w.Write(content)
	})
}

func serveLatestFirmwareBin(database *db.Database, firmwareDir string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fw, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusServiceUnavailable)
			return
		}
		path := filepath.Join(firmwareDir, fw.Version+".bin")
		f, err := os.Open(path)
		if err != nil {
			http.Error(w, "firmware file not found", http.StatusNotFound)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="firmware.bin"`)
		io.Copy(w, f)
	})
}
