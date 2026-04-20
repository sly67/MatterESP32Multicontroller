// Package fwwatch polls the firmware incoming directory every 5 s and
// auto-registers any .bin file it finds, then marks it as latest.
//
// Drop workflow:
//   1. Place <anything-v1.2.3.bin> (or any .bin) into <dataDir>/firmware/incoming/
//   2. Watcher picks it up within 5 s
//   3. File moves to <dataDir>/firmware/<version>.bin and is registered in the DB
//   4. Firmware page refreshes automatically (SSE push from the build API, or manual reload)
package fwwatch

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/karthangar/matteresp32hub/internal/db"
)

var semverRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)?)`)

// Start launches the background polling goroutine.
func Start(database *db.Database, dataDir string) {
	incomingDir := filepath.Join(dataDir, "firmware", "incoming")
	fwDir := filepath.Join(dataDir, "firmware")

	if err := os.MkdirAll(incomingDir, 0o755); err != nil {
		log.Printf("fwwatch: mkdir incoming: %v", err)
		return
	}

	go poll(database, incomingDir, fwDir)
}

func poll(database *db.Database, incomingDir, fwDir string) {
	for {
		time.Sleep(5 * time.Second)
		entries, err := os.ReadDir(incomingDir)
		if err != nil {
			log.Printf("fwwatch: readdir: %v", err)
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".bin") {
				continue
			}
			process(database, incomingDir, fwDir, name)
		}
	}
}

func process(database *db.Database, incomingDir, fwDir, name string) {
	src := filepath.Join(incomingDir, name)

	// Wait until the file is not being written to (size stable for 1 s).
	if !sizeStable(src) {
		return
	}

	version := extractVersion(name)
	dst := filepath.Join(fwDir, version+".bin")

	if err := os.Rename(src, dst); err != nil {
		log.Printf("fwwatch: rename %s → %s: %v", src, dst, err)
		return
	}

	if err := database.CreateFirmware(db.FirmwareRow{
		Version: version,
		Boards:  "esp32c3",
		Notes:   "auto-registered from incoming/",
	}); err != nil {
		log.Printf("fwwatch: db register %s: %v", version, err)
		return
	}

	if err := database.SetLatestFirmware(version); err != nil {
		log.Printf("fwwatch: set-latest %s: %v", version, err)
	}

	log.Printf("fwwatch: registered %s as version %s (set as latest)", name, version)
}

// sizeStable returns true if the file size did not change over 1 s.
func sizeStable(path string) bool {
	s1, err := os.Stat(path)
	if err != nil {
		return false
	}
	time.Sleep(1 * time.Second)
	s2, err := os.Stat(path)
	if err != nil {
		return false
	}
	return s1.Size() == s2.Size() && s1.Size() > 0
}

// extractVersion tries to parse a semver from the filename; falls back to a timestamp.
func extractVersion(filename string) string {
	base := strings.TrimSuffix(filename, ".bin")
	if m := semverRe.FindStringSubmatch(base); len(m) >= 2 {
		return m[1]
	}
	return "auto-" + time.Now().Format("20060102-150405")
}
