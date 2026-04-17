package db

import "time"

// Device represents a registered ESP32 device.
type Device struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	TemplateID string     `json:"template_id"`
	FWVersion  string     `json:"fw_version"`
	PSK        []byte     `json:"-"` // never expose in API responses
	Status     string     `json:"status"`
	LastSeen   *time.Time `json:"last_seen"`
	IP         string     `json:"ip"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateDevice inserts a new device record.
func (d *Database) CreateDevice(dev Device) error {
	_, err := d.DB.Exec(
		`INSERT INTO devices (id, name, template_id, fw_version, psk, status)
		 VALUES (?, ?, ?, ?, ?, 'unknown')`,
		dev.ID, dev.Name, dev.TemplateID, dev.FWVersion, dev.PSK)
	return err
}

// GetDevice retrieves a device by ID.
func (d *Database) GetDevice(id string) (Device, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, template_id, fw_version, psk, status, last_seen, ip, created_at
		 FROM devices WHERE id = ?`, id)
	var dev Device
	var lastSeen *time.Time
	if err := row.Scan(&dev.ID, &dev.Name, &dev.TemplateID, &dev.FWVersion,
		&dev.PSK, &dev.Status, &lastSeen, &dev.IP, &dev.CreatedAt); err != nil {
		return Device{}, err
	}
	dev.LastSeen = lastSeen
	return dev, nil
}

// ListDevices returns all devices ordered by name.
func (d *Database) ListDevices() ([]Device, error) {
	rows, err := d.DB.Query(
		`SELECT id, name, template_id, fw_version, psk, status, last_seen, ip, created_at
		 FROM devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devs []Device
	for rows.Next() {
		var dev Device
		var lastSeen *time.Time
		if err := rows.Scan(&dev.ID, &dev.Name, &dev.TemplateID, &dev.FWVersion,
			&dev.PSK, &dev.Status, &lastSeen, &dev.IP, &dev.CreatedAt); err != nil {
			return nil, err
		}
		dev.LastSeen = lastSeen
		devs = append(devs, dev)
	}
	return devs, rows.Err()
}

// UpdateDeviceStatus updates the status, IP, and last_seen for a device.
func (d *Database) UpdateDeviceStatus(id, status, ip string) error {
	_, err := d.DB.Exec(
		`UPDATE devices SET status = ?, ip = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?`,
		status, ip, id)
	return err
}
