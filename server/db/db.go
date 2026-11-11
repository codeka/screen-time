package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS families (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS family_users (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES family_users(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL UNIQUE,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS child_profiles (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    emergency_contact TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS schedule_rules (
    id TEXT PRIMARY KEY,
    child_id TEXT NOT NULL REFERENCES child_profiles(id) ON DELETE CASCADE,
    day_of_week INTEGER NOT NULL, -- 0 = Sunday, 1 = Monday, ..., 6 = Saturday
    start_time TEXT NOT NULL,      -- "15:00" (HH:MM 24h)
    end_time TEXT NOT NULL,        -- "18:00" (HH:MM 24h)
    timezone TEXT NOT NULL DEFAULT 'Local',
    enabled BOOLEAN NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    child_id TEXT NOT NULL REFERENCES child_profiles(id) ON DELETE CASCADE,
    platform TEXT NOT NULL,        -- 'WINDOWS' | 'ANDROID'
    device_name TEXT NOT NULL,
    auth_token_hash TEXT NOT NULL,
    pairing_code TEXT,             -- Temporary code (e.g. 'KT79X2')
    pairing_expires_at DATETIME,
    last_seen_at DATETIME,
    status TEXT NOT NULL DEFAULT 'LOCKED' -- 'LOCKED' | 'ACTIVE' | 'TAMPERED' | 'OFFLINE'
);

CREATE TABLE IF NOT EXISTS screen_time_requests (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    child_id TEXT NOT NULL REFERENCES child_profiles(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'PENDING', -- 'PENDING' | 'APPROVED' | 'DENIED' | 'EXPIRED'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS time_grants (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    granted_by_user_id TEXT REFERENCES family_users(id),
    request_id TEXT REFERENCES screen_time_requests(id),
    valid_from DATETIME NOT NULL,
    valid_until DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS heartbeats (
    device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    last_uptime_seconds INTEGER NOT NULL,
    last_ping_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    reported_status TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_devices_pairing ON devices(pairing_code);
CREATE INDEX IF NOT EXISTS idx_grants_device_valid ON time_grants(device_id, valid_until);
CREATE INDEX IF NOT EXISTS idx_schedules_child ON schedule_rules(child_id, day_of_week, enabled);
CREATE INDEX IF NOT EXISTS idx_requests_device_pending ON screen_time_requests(device_id, status);
`

func InitDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// modernc.org/sqlite handles pooling, but for SQLite WAL mode 1-10 connections is optimal
	database.SetMaxOpenConns(10)

	if _, err := database.Exec(schema); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	log.Printf("Initialized SQLite database successfully at: %s", dbPath)
	return database, nil
}

