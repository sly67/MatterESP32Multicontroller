package ota

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/karthangar/matteresp32hub/internal/db"
)

type contextKey struct{}

// deviceFromContext retrieves the authenticated device stored by authMiddleware.
func deviceFromContext(r *http.Request) db.Device {
	dev, _ := r.Context().Value(contextKey{}).(db.Device)
	return dev
}

const timestampTolerance = 5 * time.Minute

// signatureMessage builds the string signed by the device PSK.
// Path only (no query string) — current OTA endpoints use no query parameters.
func signatureMessage(method, path, ts string) string {
	return strings.ToUpper(method) + ":" + path + ":" + ts
}

// authenticate verifies X-Device-ID / X-Timestamp / X-HMAC headers.
// Returns the authenticated device on success, error on failure.
func authenticate(r *http.Request, database *db.Database) (db.Device, error) {
	deviceID := r.Header.Get("X-Device-ID")
	ts := r.Header.Get("X-Timestamp")
	mac := r.Header.Get("X-HMAC")

	if deviceID == "" || ts == "" || mac == "" {
		return db.Device{}, fmt.Errorf("missing auth headers")
	}

	tsUnix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return db.Device{}, fmt.Errorf("invalid X-Timestamp")
	}
	age := time.Since(time.Unix(tsUnix, 0))
	if age < 0 {
		age = -age
	}
	if age > timestampTolerance {
		return db.Device{}, fmt.Errorf("timestamp out of window")
	}

	dev, err := database.GetDevice(deviceID)
	if err != nil {
		return db.Device{}, fmt.Errorf("unknown device")
	}

	h := hmac.New(sha256.New, dev.PSK)
	h.Write([]byte(signatureMessage(r.Method, r.URL.Path, ts)))
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(mac)) {
		return db.Device{}, fmt.Errorf("HMAC mismatch")
	}

	return dev, nil
}
