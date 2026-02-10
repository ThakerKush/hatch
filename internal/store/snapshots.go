package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Snapshot holds metadata about a persisted VM snapshot stored in S3.
type Snapshot struct {
	ID        string    `json:"id"`
	VMID      string    `json:"vm_id"`
	StateKey  string    `json:"state_key"`
	MemoryKey string    `json:"memory_key"`
	DiskKey   string    `json:"disk_key"`
	VMConfig  string    `json:"vm_config"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateSnapshot inserts a new snapshot record.
func (d *DB) CreateSnapshot(s Snapshot) error {
	_, err := d.db.Exec(
		`INSERT INTO snapshots (id, vm_id, state_key, memory_key, disk_key, vm_config, size_bytes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		s.ID, s.VMID, s.StateKey, s.MemoryKey, s.DiskKey, s.VMConfig, s.SizeBytes, s.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	return nil
}

// GetSnapshot retrieves a snapshot by ID. Returns nil if not found.
func (d *DB) GetSnapshot(id string) (*Snapshot, error) {
	row := d.db.QueryRow(
		`SELECT id, vm_id, state_key, memory_key, disk_key, vm_config, size_bytes, created_at
		 FROM snapshots WHERE id = $1`, id,
	)
	return scanSnapshot(row)
}

// GetLatestSnapshot returns the most recent snapshot for a VM. Returns nil if none.
func (d *DB) GetLatestSnapshot(vmID string) (*Snapshot, error) {
	row := d.db.QueryRow(
		`SELECT id, vm_id, state_key, memory_key, disk_key, vm_config, size_bytes, created_at
		 FROM snapshots WHERE vm_id = $1 ORDER BY created_at DESC LIMIT 1`, vmID,
	)
	return scanSnapshot(row)
}

// ListSnapshots returns all snapshots for a given VM.
func (d *DB) ListSnapshots(vmID string) ([]Snapshot, error) {
	rows, err := d.db.Query(
		`SELECT id, vm_id, state_key, memory_key, disk_key, vm_config, size_bytes, created_at
		 FROM snapshots WHERE vm_id = $1 ORDER BY created_at DESC`, vmID,
	)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.VMID, &s.StateKey, &s.MemoryKey, &s.DiskKey,
			&s.VMConfig, &s.SizeBytes, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// DeleteSnapshot removes a snapshot by ID. Returns true if a row was deleted.
func (d *DB) DeleteSnapshot(id string) (bool, error) {
	res, err := d.db.Exec(`DELETE FROM snapshots WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete snapshot: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteSnapshotsByVM removes all snapshots for a VM.
func (d *DB) DeleteSnapshotsByVM(vmID string) error {
	_, err := d.db.Exec(`DELETE FROM snapshots WHERE vm_id = $1`, vmID)
	if err != nil {
		return fmt.Errorf("delete snapshots for vm: %w", err)
	}
	return nil
}

func scanSnapshot(row *sql.Row) (*Snapshot, error) {
	s := &Snapshot{}
	err := row.Scan(&s.ID, &s.VMID, &s.StateKey, &s.MemoryKey, &s.DiskKey,
		&s.VMConfig, &s.SizeBytes, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan snapshot: %w", err)
	}
	return s, nil
}
