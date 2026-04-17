package db

import "time"

// OTALogRow is one OTA update event.
type OTALogRow struct {
	ID        int64     `json:"id"`
	DeviceID  string    `json:"device_id"`
	FromVer   string    `json:"from_ver"`
	ToVer     string    `json:"to_ver"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateOTALog inserts an OTA event. Result is typically "ok" or "error".
func (d *Database) CreateOTALog(entry OTALogRow) error {
	_, err := d.DB.Exec(
		`INSERT INTO ota_log (device_id, from_ver, to_ver, result) VALUES (?, ?, ?, ?)`,
		entry.DeviceID, entry.FromVer, entry.ToVer, entry.Result)
	return err
}

// ListOTALogForDevice returns the 20 most recent OTA events for a device.
func (d *Database) ListOTALogForDevice(deviceID string) ([]OTALogRow, error) {
	rows, err := d.DB.Query(
		`SELECT id, device_id, from_ver, to_ver, result, created_at
		 FROM ota_log WHERE device_id = ? ORDER BY created_at DESC LIMIT 20`,
		deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OTALogRow
	for rows.Next() {
		var r OTALogRow
		if err := rows.Scan(&r.ID, &r.DeviceID, &r.FromVer, &r.ToVer, &r.Result, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
