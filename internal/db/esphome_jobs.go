package db

import (
	"database/sql"
	"time"
)

// ESPHomeJob is a row in the esphome_jobs table.
type ESPHomeJob struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	ConfigJSON string    `json:"config_json"`
	Status     string    `json:"status"`
	Log        string    `json:"log"`
	BinaryPath string    `json:"binary_path"`
	Error      string    `json:"error"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateJob inserts a new job row.
func (d *Database) CreateJob(job ESPHomeJob) error {
	_, err := d.DB.Exec(
		`INSERT INTO esphome_jobs (id, device_name, config_json, status)
		 VALUES (?, ?, ?, ?)`,
		job.ID, job.DeviceName, job.ConfigJSON, job.Status)
	return err
}

// GetJob retrieves a job by ID.
func (d *Database) GetJob(id string) (ESPHomeJob, error) {
	row := d.DB.QueryRow(
		`SELECT id, COALESCE(device_id,''), device_name, config_json, status,
		        log, binary_path, error, created_at, updated_at
		 FROM esphome_jobs WHERE id = ?`, id)
	var j ESPHomeJob
	err := row.Scan(&j.ID, &j.DeviceID, &j.DeviceName, &j.ConfigJSON, &j.Status,
		&j.Log, &j.BinaryPath, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return ESPHomeJob{}, err
	}
	return j, nil
}

// ListJobs returns all jobs ordered by created_at descending.
func (d *Database) ListJobs() ([]ESPHomeJob, error) {
	rows, err := d.DB.Query(
		`SELECT id, COALESCE(device_id,''), device_name, config_json, status,
		        log, binary_path, error, created_at, updated_at
		 FROM esphome_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []ESPHomeJob
	for rows.Next() {
		var j ESPHomeJob
		if err := rows.Scan(&j.ID, &j.DeviceID, &j.DeviceName, &j.ConfigJSON, &j.Status,
			&j.Log, &j.BinaryPath, &j.Error, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus sets status, optionally binary_path and error.
func (d *Database) UpdateJobStatus(id, status, binaryPath, errMsg string) error {
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET status = ?, binary_path = ?, error = ?,
		 updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, binaryPath, errMsg, id)
	return err
}

// UpdateJobDone marks a job done and records the binary path and device_id.
func (d *Database) UpdateJobDone(id, binaryPath, deviceID string) error {
	var devID interface{}
	if deviceID != "" {
		devID = deviceID
	}
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET status = 'done', binary_path = ?, device_id = ?,
		 updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		binaryPath, devID, id)
	return err
}

// AppendJobLog appends a log line (newline-separated) to the job's log field.
func (d *Database) AppendJobLog(id, line string) error {
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET
		   log = CASE WHEN log = '' THEN ? ELSE log || char(10) || ? END,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		line, line, id)
	return err
}

// ResetStaleJobs marks pending/running jobs as failed (called on hub startup).
func (d *Database) ResetStaleJobs() error {
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET status = 'failed', error = 'hub restarted',
		 updated_at = CURRENT_TIMESTAMP WHERE status IN ('pending','running')`)
	return err
}

// DeleteOldJobs removes done/failed/cancelled jobs created before cutoff.
func (d *Database) DeleteOldJobs(cutoff time.Time) error {
	_, err := d.DB.Exec(
		`DELETE FROM esphome_jobs WHERE status IN ('done','failed','cancelled')
		 AND created_at < ?`, cutoff)
	return err
}

// GetLatestJobForDevice returns the most recent job for a device_id (for Fleet badge).
func (d *Database) GetLatestJobForDevice(deviceID string) (ESPHomeJob, error) {
	row := d.DB.QueryRow(
		`SELECT id, COALESCE(device_id,''), device_name, config_json, status,
		        log, binary_path, error, created_at, updated_at
		 FROM esphome_jobs WHERE device_id = ? ORDER BY created_at DESC LIMIT 1`, deviceID)
	var j ESPHomeJob
	err := row.Scan(&j.ID, &j.DeviceID, &j.DeviceName, &j.ConfigJSON, &j.Status,
		&j.Log, &j.BinaryPath, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return ESPHomeJob{}, nil
	}
	return j, err
}
