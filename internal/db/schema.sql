CREATE TABLE IF NOT EXISTS devices (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    template_id TEXT NOT NULL,
    fw_version  TEXT NOT NULL DEFAULT '',
    psk         BLOB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'unknown',
    last_seen   DATETIME,
    ip          TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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
    boards     TEXT NOT NULL,
    notes      TEXT NOT NULL DEFAULT '',
    is_latest  INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS flash_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  TEXT NOT NULL,
    result     TEXT NOT NULL,
    error      TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ota_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  TEXT NOT NULL,
    from_ver   TEXT NOT NULL,
    to_ver     TEXT NOT NULL,
    result     TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
