// Package usb provides USB serial port detection and esptool wrapping.
package usb

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Port describes a detected USB serial port.
type Port struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// ListPorts returns all available USB serial ports on the host.
// Scans /dev/ttyUSB* and /dev/ttyACM* (Linux).
func ListPorts() ([]Port, error) {
	var ports []Port
	for _, pattern := range []string{"/dev/ttyUSB*", "/dev/ttyACM*"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if _, err := os.Stat(m); err == nil {
				ports = append(ports, Port{
					Path: m,
					Name: strings.TrimPrefix(m, "/dev/"),
				})
			}
		}
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].Path < ports[j].Path })
	return ports, nil
}
