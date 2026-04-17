package usb

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultBaud = 460800

// ChipInfo holds basic info read from a connected ESP32.
type ChipInfo struct {
	ChipType string
	MacAddr  string
	DeviceID string // "esp-AABBCC" from last 3 MAC bytes
}

// GetChipInfo reads chip ID and MAC from a connected device.
func GetChipInfo(port string) (ChipInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx,
		"esptool.py", "--port", port, "--baud", fmt.Sprintf("%d", defaultBaud),
		"chip_id").CombinedOutput()
	if err != nil {
		return ChipInfo{}, fmt.Errorf("esptool chip_id: %w\n%s", err, out)
	}
	return parseChipInfo(string(out))
}

func parseChipInfo(out string) (ChipInfo, error) {
	var info ChipInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Chip is ") {
			info.ChipType = strings.TrimPrefix(line, "Chip is ")
		}
		if strings.Contains(line, "MAC:") {
			parts := strings.SplitN(line, "MAC:", 2)
			if len(parts) == 2 {
				mac := strings.TrimSpace(parts[1])
				info.MacAddr = mac
				segs := strings.Split(mac, ":")
				if len(segs) >= 3 {
					suffix := strings.Join(segs[len(segs)-3:], "")
					info.DeviceID = "esp-" + strings.ToUpper(suffix)
				}
			}
		}
	}
	if info.DeviceID == "" {
		return info, fmt.Errorf("could not parse MAC from esptool output")
	}
	return info, nil
}

// WriteFlash writes a binary image to the given address on a connected device.
func WriteFlash(port, binPath string, addr uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	addrStr := fmt.Sprintf("0x%x", addr)
	out, err := exec.CommandContext(ctx,
		"esptool.py", "--port", port, "--baud", fmt.Sprintf("%d", defaultBaud),
		"write_flash", addrStr, binPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("esptool write_flash at %s: %w\n%s", addrStr, err, out)
	}
	return nil
}

// EraseFlash erases the entire flash of a connected device.
func EraseFlash(port string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx,
		"esptool.py", "--port", port, "--baud", fmt.Sprintf("%d", defaultBaud),
		"erase_flash").CombinedOutput()
	if err != nil {
		return fmt.Errorf("esptool erase_flash: %w\n%s", err, out)
	}
	return nil
}
