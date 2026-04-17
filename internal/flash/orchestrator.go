// Package flash orchestrates the full ESP32 device flashing workflow:
// read chip ID → generate PSK → compile NVS → generate NVS binary →
// write firmware → write NVS → register device in DB.
package flash

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/nvs"
	"github.com/karthangar/matteresp32hub/internal/usb"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// nvsFlashAddr is the NVS partition address in the standard ESP-IDF partition table.
const nvsFlashAddr = 0x9000

// Request holds the parameters for flashing a single device.
type Request struct {
	Port         string
	Template     *yamldef.Template
	DeviceName   string // e.g. "1/Bedroom"
	WiFiSSID     string
	WiFiPassword string
	FirmwarePath string // absolute path to .bin on server
	FWVersion    string
}

// Result is the outcome of a single flash operation.
type Result struct {
	DeviceID string
	Name     string
	Error    error
}

// FlashDevice performs the full flash sequence for one device.
func FlashDevice(database *db.Database, req Request) Result {
	// 1. Read chip info (device ID from MAC)
	chip, err := usb.GetChipInfo(req.Port)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("chip_id: %w", err)}
	}

	// 2. Generate PSK
	psk := make([]byte, 32)
	if _, err := rand.Read(psk); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("generate PSK: %w", err)}
	}

	// 3. Generate Matter discriminator + passcode
	discrim, passcode, err := generateMatterCreds()
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("generate matter creds: %w", err)}
	}

	// 4. Compile NVS CSV
	devCfg := nvs.DeviceConfig{
		Name:           req.DeviceName,
		WiFiSSID:       req.WiFiSSID,
		WiFiPassword:   req.WiFiPassword,
		PSK:            psk,
		BoardID:        req.Template.Board,
		MatterDiscrim:  discrim,
		MatterPasscode: passcode,
	}
	csv, err := nvs.Compile(req.Template, devCfg)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("compile NVS: %w", err)}
	}

	// 5. Generate NVS binary
	binPath, err := nvs.GenerateBinary(csv)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("generate NVS binary: %w", err)}
	}
	defer os.RemoveAll(filepath.Dir(binPath))

	// 6. Flash firmware
	if err := usb.WriteFlash(req.Port, req.FirmwarePath, 0x0); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("flash firmware: %w", err)}
	}

	// 7. Flash NVS
	if err := usb.WriteFlash(req.Port, binPath, nvsFlashAddr); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("flash NVS: %w", err)}
	}

	// 8. Register device in DB
	if err := database.CreateDevice(db.Device{
		ID:         chip.DeviceID,
		Name:       req.DeviceName,
		TemplateID: req.Template.ID,
		FWVersion:  req.FWVersion,
		PSK:        psk,
		Status:     "unknown",
	}); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("register device: %w", err)}
	}

	return Result{DeviceID: chip.DeviceID, Name: req.DeviceName}
}

// generateMatterCreds returns a random valid discriminator and passcode.
func generateMatterCreds() (uint16, uint32, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, 0, err
	}
	discrim := uint16(binary.LittleEndian.Uint16(buf[0:2])) & 0x0FFF

	invalidPasscodes := map[uint32]bool{
		0: true, 11111111: true, 22222222: true, 33333333: true,
		44444444: true, 55555555: true, 66666666: true, 77777777: true,
		88888888: true, 99999999: true, 12345678: true, 87654321: true,
	}
	passcode := binary.LittleEndian.Uint32(buf[2:6])&0x07FFFFFF + 1
	if invalidPasscodes[passcode] {
		passcode = 20202021
	}
	return discrim, passcode, nil
}
