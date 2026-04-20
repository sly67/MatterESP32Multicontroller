package flash

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/usb"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// ESPHomeRequest holds the parameters for an ESPHome flash operation.
type ESPHomeRequest struct {
	Ctx          context.Context
	Port         string
	DeviceName   string
	WiFiSSID     string
	WiFiPassword string
	Board        string
	HAIntegration bool
	Components   []esphome.ComponentConfig
}

// FlashESPHomeDevice runs the full ESPHome flash sequence.
// Log lines from the ESPHome compiler are written to logWriter.
// modules is the map from module ID → *yamldef.Module, loaded from the library.
func FlashESPHomeDevice(database *db.Database, builder *esphome.Builder,
	modules map[string]*yamldef.Module, req ESPHomeRequest, logWriter io.Writer) Result {

	// 1. Read chip info
	chip, err := usb.GetChipInfo(req.Port)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("chip_id: %w", err)}
	}

	// 2. Generate OTA password
	otaBuf := make([]byte, 16)
	if _, err := rand.Read(otaBuf); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("generate OTA password: %w", err)}
	}
	otaPassword := hex.EncodeToString(otaBuf)

	// 3. Optionally generate HA API key
	var apiKey string
	if req.HAIntegration {
		keyBuf := make([]byte, 32)
		if _, err := rand.Read(keyBuf); err != nil {
			return Result{Name: req.DeviceName, Error: fmt.Errorf("generate API key: %w", err)}
		}
		apiKey = base64.StdEncoding.EncodeToString(keyBuf)
	}

	// 4. Assemble ESPHome YAML
	cfg := esphome.Config{
		Board:         req.Board,
		DeviceName:    req.DeviceName,
		DeviceID:      chip.DeviceID,
		WiFiSSID:      req.WiFiSSID,
		WiFiPassword:  req.WiFiPassword,
		HAIntegration: req.HAIntegration,
		APIKey:        apiKey,
		OTAPassword:   otaPassword,
		Components:    req.Components,
	}
	yamlStr, err := esphome.Assemble(cfg, modules)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("assemble YAML: %w", err)}
	}

	// 5. Compile via Docker
	bin, err := builder.Compile(req.Ctx, req.DeviceName, yamlStr, logWriter)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("compile: %w", err)}
	}

	// 6. Write binary to temp file and flash at 0x0
	tmpDir, err := os.MkdirTemp("", "esphome-flash-*")
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("temp dir: %w", err)}
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "firmware-factory.bin")
	if err := os.WriteFile(binPath, bin, 0644); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("write firmware: %w", err)}
	}
	if err := usb.WriteFlash(req.Port, binPath, 0x0); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("flash firmware: %w", err)}
	}

	// 7. Persist ESPHome config as JSON
	esphomeCfg := struct {
		Board         string                    `json:"board"`
		HAIntegration bool                      `json:"ha_integration"`
		OTAPassword   string                    `json:"ota_password"`
		Components    []esphome.ComponentConfig `json:"components"`
	}{
		Board:         req.Board,
		HAIntegration: req.HAIntegration,
		OTAPassword:   otaPassword,
		Components:    req.Components,
	}
	cfgJSON, err := json.Marshal(esphomeCfg)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("marshal config: %w", err)}
	}

	// 8. Register device in DB
	if err := database.CreateDevice(db.Device{
		ID:            chip.DeviceID,
		Name:          req.DeviceName,
		FirmwareType:  "esphome",
		ESPHomeConfig: string(cfgJSON),
		ESPHomeAPIKey: apiKey,
		PSK:           []byte{},
	}); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("register device: %w", err)}
	}

	return Result{DeviceID: chip.DeviceID, Name: req.DeviceName}
}
