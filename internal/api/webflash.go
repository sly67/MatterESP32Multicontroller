package api

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	godata "github.com/karthangar/matteresp32hub/data"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/matter"
	"github.com/karthangar/matteresp32hub/internal/nvs"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// preparedSession holds an NVS binary and firmware version keyed by token.
type preparedSession struct {
	nvsBin    []byte
	fwVersion string
	createdAt time.Time
}

var (
	sessionMu   sync.Mutex
	sessions    = map[string]*preparedSession{}
	sessionOnce sync.Once
)

// startSessionGC removes sessions older than 30 minutes.
func startSessionGC() {
	sessionOnce.Do(func() {
		go func() {
			for {
				time.Sleep(5 * time.Minute)
				sessionMu.Lock()
				for k, s := range sessions {
					if time.Since(s.createdAt) > 30*time.Minute {
						delete(sessions, k)
					}
				}
				sessionMu.Unlock()
			}
		}()
	})
}

func webflashRouter(cfg *config.Config, database *db.Database) func(chi.Router) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	firmwareDir := filepath.Join(dataDir, "firmware")

	startSessionGC()

	return func(r chi.Router) {
		// Static manifest (no NVS — kept for backward compat)
		r.Get("/manifest.json", serveWebFlashManifest(database))
		// Dynamic manifest with NVS (requires ?token=)
		r.Get("/manifest", serveWebFlashManifestDynamic(database))
		r.Get("/nvs", serveWebFlashNVS())
		r.Post("/prepare", prepareWebFlash(database))

		r.Get("/bootloader.bin", serveFlashStatic("flash/esp32c3/bootloader.bin", "bootloader.bin"))
		r.Get("/partition-table.bin", serveFlashStatic("flash/esp32c3/partition-table.bin", "partition-table.bin"))
		r.Get("/ota_data_initial.bin", serveFlashStatic("flash/esp32c3/ota_data_initial.bin", "ota_data_initial.bin"))
		r.Get("/firmware.bin", serveLatestFirmwareBin(database, firmwareDir))
	}
}

// prepareWebFlash generates PSK + Matter creds + NVS binary, stores with token.
func prepareWebFlash(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TemplateID   string `json:"template_id"`
			DeviceName   string `json:"device_name"`
			WiFiSSID     string `json:"wifi_ssid"`
			WiFiPassword string `json:"wifi_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.TemplateID == "" || req.DeviceName == "" {
			http.Error(w, "template_id and device_name are required", http.StatusBadRequest)
			return
		}

		tplRow, err := database.GetTemplate(req.TemplateID)
		if err != nil {
			http.Error(w, "template not found", http.StatusNotFound)
			return
		}
		tpl, err := yamldef.ParseTemplate([]byte(tplRow.YAMLBody))
		if err != nil {
			http.Error(w, "template parse error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fw, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusServiceUnavailable)
			return
		}

		psk := make([]byte, 32)
		if _, err := rand.Read(psk); err != nil {
			http.Error(w, "psk: "+err.Error(), http.StatusInternalServerError)
			return
		}
		discrim, passcode, err := webflashMatterCreds()
		if err != nil {
			http.Error(w, "matter creds: "+err.Error(), http.StatusInternalServerError)
			return
		}

		csv, err := nvs.Compile(tpl, nvs.DeviceConfig{
			Name:           req.DeviceName,
			WiFiSSID:       req.WiFiSSID,
			WiFiPassword:   req.WiFiPassword,
			PSK:            psk,
			BoardID:        tpl.Board,
			MatterDiscrim:  discrim,
			MatterPasscode: passcode,
		})
		if err != nil {
			http.Error(w, "nvs compile: "+err.Error(), http.StatusInternalServerError)
			return
		}

		binPath, err := nvs.GenerateBinary(csv)
		if err != nil {
			http.Error(w, "nvs binary: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer os.RemoveAll(filepath.Dir(binPath))

		binData, err := os.ReadFile(binPath)
		if err != nil {
			http.Error(w, "read nvs bin: "+err.Error(), http.StatusInternalServerError)
			return
		}

		token, err := randomHex(16)
		if err != nil {
			http.Error(w, "token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		sessionMu.Lock()
		sessions[token] = &preparedSession{
			nvsBin:    binData,
			fwVersion: fw.Version,
			createdAt: time.Now(),
		}
		sessionMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":         token,
			"discriminator": discrim,
			"passcode":      passcode,
			"qr_payload":   matter.SetupQRPayload(discrim, passcode),
		})
	}
}

func serveWebFlashManifestDynamic(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")

		sessionMu.Lock()
		sess, ok := sessions[token]
		sessionMu.Unlock()

		if !ok {
			http.Error(w, "invalid or expired token", http.StatusBadRequest)
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
			Version: sess.fwVersion,
			Builds: []build{
				{
					ChipFamily: "ESP32-C3",
					Parts: []part{
						{Path: "/api/webflash/bootloader.bin", Offset: 0x0},
						{Path: "/api/webflash/partition-table.bin", Offset: 0x8000},
						{Path: "/api/webflash/ota_data_initial.bin", Offset: 0xf000},
						{Path: "/api/webflash/firmware.bin", Offset: 0x20000},
						{Path: fmt.Sprintf("/api/webflash/nvs?token=%s", token), Offset: 0x9000},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}
}

func serveWebFlashNVS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")

		sessionMu.Lock()
		sess, ok := sessions[token]
		sessionMu.Unlock()

		if !ok {
			http.Error(w, "invalid or expired token", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="nvs.bin"`)
		w.Write(sess.nvsBin)
	}
}

func webflashMatterCreds() (uint16, uint32, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, 0, err
	}
	discrim := binary.LittleEndian.Uint16(buf[0:2]) & 0x0FFF
	passcode := binary.LittleEndian.Uint32(buf[2:6])
	passcode = passcode%(99999998) + 1
	return discrim, passcode, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
