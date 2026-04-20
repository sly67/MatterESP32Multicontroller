package api

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	godata "github.com/karthangar/matteresp32hub/data"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/library"
	"github.com/karthangar/matteresp32hub/internal/matter"
	"github.com/karthangar/matteresp32hub/internal/nvs"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// preparedSession holds flash data keyed by token.
// For Matter sessions: nvsBin + fwVersion are set.
// For ESPHome sessions: espBin + espBoard are set.
type preparedSession struct {
	nvsBin    []byte
	fwVersion string
	espBin    []byte
	espBoard  string
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
		r.Get("/firmware", serveSessionFirmwareBin(firmwareDir))
		r.Post("/prepare", prepareWebFlash(database))

		// ESPHome browser flash
		r.Post("/esphome-prepare", prepareWebFlashESPHome(database, dataDir))
		r.Get("/esphome-manifest", serveWebFlashESPHomeManifest())
		r.Get("/esphome-firmware", serveWebFlashESPHomeFirmware())

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
			FWVersion    string `json:"fw_version"`
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

		var fw db.FirmwareRow
		var fwErr error
		if req.FWVersion != "" {
			fw, fwErr = database.GetFirmware(req.FWVersion)
		} else {
			fw, fwErr = database.GetLatestFirmware()
		}
		if fwErr != nil {
			http.Error(w, "firmware not found", http.StatusNotFound)
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
						{Path: fmt.Sprintf("/api/webflash/firmware?token=%s", token), Offset: 0x20000},
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

// safeVersion rejects version strings that could be used for path traversal.
var safeVersion = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func serveLatestFirmwareBin(database *db.Database, firmwareDir string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fw, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusServiceUnavailable)
			return
		}
		if !safeVersion.MatchString(fw.Version) {
			http.Error(w, "invalid firmware version", http.StatusInternalServerError)
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

// boardToChipFamily maps ESPHome board identifiers to esp-web-tools chip families.
func boardToChipFamily(board string) string {
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
		return "ESP32"
	}
}

// prepareWebFlashESPHome compiles ESPHome firmware for browser flashing.
// Streams ndjson log lines during compile, then emits a final {ok,token} or {ok:false,error} line.
func prepareWebFlashESPHome(database *db.Database, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Board         string                    `json:"board"`
			Components    []esphome.ComponentConfig  `json:"components"`
			DeviceName    string                    `json:"device_name"`
			WiFiSSID      string                    `json:"wifi_ssid"`
			WiFiPassword  string                    `json:"wifi_password"`
			HAIntegration bool                      `json:"ha_integration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.DeviceName == "" || req.Board == "" {
			http.Error(w, "device_name and board are required", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Transfer-Encoding", "chunked")
		flusher, canFlush := w.(http.Flusher)

		sendLine := func(v interface{}) {
			json.NewEncoder(w).Encode(v) //nolint:errcheck
			if canFlush {
				flusher.Flush()
			}
		}

		mods, err := library.LoadModules()
		if err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "load modules: " + err.Error()})
			return
		}
		modMap := make(map[string]*yamldef.Module, len(mods))
		for _, m := range mods {
			modMap[m.ID] = m
		}

		deviceID, err := randomHex(6)
		if err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "device id: " + err.Error()})
			return
		}

		otaBuf := make([]byte, 16)
		if _, err := rand.Read(otaBuf); err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "ota password: " + err.Error()})
			return
		}
		otaPassword := hex.EncodeToString(otaBuf)

		var apiKey string
		if req.HAIntegration {
			keyBuf := make([]byte, 32)
			if _, err := rand.Read(keyBuf); err != nil {
				sendLine(map[string]interface{}{"ok": false, "error": "api key: " + err.Error()})
				return
			}
			apiKey = base64.StdEncoding.EncodeToString(keyBuf)
		}

		yamlStr, err := esphome.Assemble(esphome.Config{
			Board:         req.Board,
			DeviceName:    req.DeviceName,
			DeviceID:      deviceID,
			WiFiSSID:      req.WiFiSSID,
			WiFiPassword:  req.WiFiPassword,
			HAIntegration: req.HAIntegration,
			APIKey:        apiKey,
			OTAPassword:   otaPassword,
			Components:    req.Components,
		}, modMap)
		if err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "assemble YAML: " + err.Error()})
			return
		}

		builder, err := esphome.NewBuilder(dataDir+"/esphome-cache", os.Getenv("ESPHOME_CACHE_VOLUME"))
		if err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "builder: " + err.Error()})
			return
		}
		defer builder.Close()

		pr, pw := io.Pipe()
		type binResult struct {
			bin []byte
			err error
		}
		ch := make(chan binResult, 1)
		go func() {
			bin, err := builder.Compile(r.Context(), req.DeviceName, yamlStr, pw)
			pw.Close()
			ch <- binResult{bin, err}
		}()

		// Keepalive: send a ping every 20s when the compiler produces no output,
		// preventing browser/proxy timeouts during silent phases (e.g. dep download).
		lineCh := make(chan string)
		go func() {
			scanner := bufio.NewScanner(pr)
			for scanner.Scan() {
				lineCh <- scanner.Text()
			}
			close(lineCh)
		}()
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
	scanLoop:
		for {
			select {
			case line, ok := <-lineCh:
				if !ok {
					break scanLoop
				}
				if s := strings.TrimSpace(line); s != "" {
					sendLine(map[string]string{"log": s})
				}
				ticker.Reset(20 * time.Second)
			case <-ticker.C:
				sendLine(map[string]string{"log": "…"})
			}
		}

		res := <-ch
		if res.err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "compile: " + res.err.Error()})
			return
		}

		cfgJSON, _ := json.Marshal(struct {
			Board         string                    `json:"board"`
			HAIntegration bool                      `json:"ha_integration"`
			OTAPassword   string                    `json:"ota_password"`
			Components    []esphome.ComponentConfig `json:"components"`
		}{req.Board, req.HAIntegration, otaPassword, req.Components})

		if err := database.CreateDevice(db.Device{
			ID:            deviceID,
			Name:          req.DeviceName,
			FirmwareType:  "esphome",
			ESPHomeConfig: string(cfgJSON),
			ESPHomeAPIKey: apiKey,
			PSK:           []byte{},
		}); err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "register device: " + err.Error()})
			return
		}

		token, err := randomHex(16)
		if err != nil {
			sendLine(map[string]interface{}{"ok": false, "error": "token: " + err.Error()})
			return
		}
		sessionMu.Lock()
		sessions[token] = &preparedSession{
			espBin:    res.bin,
			espBoard:  req.Board,
			createdAt: time.Now(),
		}
		sessionMu.Unlock()

		sendLine(map[string]interface{}{"ok": true, "token": token, "device_id": deviceID})
	}
}

func serveWebFlashESPHomeManifest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		sessionMu.Lock()
		sess, ok := sessions[token]
		sessionMu.Unlock()
		if !ok || len(sess.espBin) == 0 {
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
			Name:    "ESPHome Firmware",
			Version: "esphome",
			Builds: []build{{
				ChipFamily: boardToChipFamily(sess.espBoard),
				Parts:      []part{{Path: fmt.Sprintf("/api/webflash/esphome-firmware?token=%s", token), Offset: 0x0}},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest) //nolint:errcheck
	}
}

func serveWebFlashESPHomeFirmware() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		sessionMu.Lock()
		sess, ok := sessions[token]
		sessionMu.Unlock()
		if !ok || len(sess.espBin) == 0 {
			http.Error(w, "invalid or expired token", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="firmware-factory.bin"`)
		w.Write(sess.espBin) //nolint:errcheck
	}
}

func serveSessionFirmwareBin(firmwareDir string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")

		sessionMu.Lock()
		sess, ok := sessions[token]
		sessionMu.Unlock()

		if !ok {
			http.Error(w, "invalid or expired token", http.StatusBadRequest)
			return
		}
		if !safeVersion.MatchString(sess.fwVersion) {
			http.Error(w, "invalid firmware version", http.StatusInternalServerError)
			return
		}

		path := filepath.Join(firmwareDir, sess.fwVersion+".bin")
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
