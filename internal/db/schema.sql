CREATE TABLE IF NOT EXISTS devices (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    template_id     TEXT,
    fw_version      TEXT NOT NULL DEFAULT '',
    psk             BLOB NOT NULL DEFAULT x'',
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_seen       DATETIME,
    ip              TEXT NOT NULL DEFAULT '',
    matter_discrim  INTEGER NOT NULL DEFAULT 0,
    matter_passcode INTEGER NOT NULL DEFAULT 0,
    firmware_type   TEXT NOT NULL DEFAULT 'matter',
    esphome_config  TEXT NOT NULL DEFAULT '',
    esphome_api_key TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS templates (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    board      TEXT NOT NULL,
    yaml_body  TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS modules (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    category   TEXT NOT NULL,
    builtin    INTEGER NOT NULL DEFAULT 0,
    yaml_body  TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS effects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    builtin    INTEGER NOT NULL DEFAULT 0,
    yaml_body  TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS firmware (
    version    TEXT PRIMARY KEY,
    boards     TEXT NOT NULL,  -- comma-separated board IDs e.g. "esp32-c3,esp32-h2"
    notes      TEXT NOT NULL DEFAULT '',
    is_latest  INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS flash_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    result     TEXT NOT NULL,
    error      TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ota_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    from_ver   TEXT NOT NULL,
    to_ver     TEXT NOT NULL,
    result     TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_devices_name ON devices(name);

CREATE TABLE IF NOT EXISTS esphome_jobs (
    id          TEXT PRIMARY KEY,
    device_id   TEXT,
    device_name TEXT NOT NULL,
    config_json TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    log         TEXT NOT NULL DEFAULT '',
    binary_path TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_esphome_jobs_device ON esphome_jobs(device_id);
CREATE INDEX IF NOT EXISTS idx_esphome_jobs_created ON esphome_jobs(created_at DESC);
