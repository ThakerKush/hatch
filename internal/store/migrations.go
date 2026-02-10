package store

import "log/slog"

const schemaV1 = `
CREATE TABLE IF NOT EXISTS images (
    id          TEXT PRIMARY KEY,
    kernel_path TEXT NOT NULL,
    rootfs_path TEXT NOT NULL,
    boot_args   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS templates (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    image_id    TEXT NOT NULL REFERENCES images(id),
    user_data   TEXT NOT NULL DEFAULT '',
    vcpu_count  INTEGER NOT NULL DEFAULT 1,
    mem_mib     INTEGER NOT NULL DEFAULT 256,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS vms (
    id              TEXT PRIMARY KEY,
    image_id        TEXT NOT NULL REFERENCES images(id),
    template_id     TEXT REFERENCES templates(id),
    state           TEXT NOT NULL DEFAULT 'starting',
    vcpu_count      INTEGER NOT NULL,
    mem_mib         INTEGER NOT NULL,
    guest_ip        TEXT,
    guest_mac       TEXT,
    tap_name        TEXT,
    socket_path     TEXT,
    work_dir        TEXT,
    user_data       TEXT NOT NULL DEFAULT '',
    enable_network  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS snapshots (
    id          TEXT PRIMARY KEY,
    vm_id       TEXT NOT NULL REFERENCES vms(id),
    state_key   TEXT NOT NULL,
    memory_key  TEXT NOT NULL,
    disk_key    TEXT NOT NULL,
    vm_config   TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS proxy_routes (
    id          TEXT PRIMARY KEY,
    vm_id       TEXT NOT NULL REFERENCES vms(id),
    subdomain   TEXT NOT NULL UNIQUE,
    target_port INTEGER NOT NULL,
    auto_wake   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

func (d *DB) migrate() error {
	slog.Info("running database migrations")
	_, err := d.db.Exec(schemaV1)
	return err
}
