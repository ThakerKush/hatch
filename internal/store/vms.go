package store

import (
	"database/sql"
	"fmt"
	"time"
)

// VM state constants.
const (
	VMStateStarting    = "starting"
	VMStateRunning     = "running"
	VMStateStopping    = "stopping"
	VMStateStopped     = "stopped"
	VMStateSnapshotted = "snapshotted"
	VMStateError       = "error"
)

// VM represents a micro-VM instance.
type VM struct {
	ID            string    `json:"id"`
	ImageID       string    `json:"image_id"`
	TemplateID    string    `json:"template_id,omitempty"`
	UserID        string    `json:"user_id,omitempty"`
	State         string    `json:"state"`
	VCPUCount     int       `json:"vcpu_count"`
	MemMib        int       `json:"mem_mib"`
	GuestIP       string    `json:"guest_ip,omitempty"`
	GuestMAC      string    `json:"guest_mac,omitempty"`
	TapName       string    `json:"tap_name,omitempty"`
	SSHPort       int       `json:"ssh_port,omitempty"`
	SocketPath    string    `json:"socket_path,omitempty"`
	WorkDir       string    `json:"-"`
	UserData      string    `json:"user_data,omitempty"`
	EnableNetwork bool      `json:"enable_network"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// vmColumns is the SELECT column list shared by all VM queries.
const vmColumns = `id, image_id, COALESCE(template_id,''), state, vcpu_count, mem_mib,
  COALESCE(guest_ip,''), COALESCE(guest_mac,''), COALESCE(tap_name,''), COALESCE(ssh_port,0),
  COALESCE(socket_path,''), COALESCE(work_dir,''), user_data, COALESCE(user_id,''),
  enable_network, created_at, updated_at`

// CreateVM inserts a new VM record.
func (d *DB) CreateVM(vm VM) error {
	_, err := d.db.Exec(
		`INSERT INTO vms (id, image_id, template_id, state, vcpu_count, mem_mib,
		  guest_ip, guest_mac, tap_name, ssh_port, socket_path, work_dir, user_data, user_id,
		  enable_network, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		vm.ID, vm.ImageID, nullStr(vm.TemplateID), vm.State, vm.VCPUCount, vm.MemMib,
		vm.GuestIP, vm.GuestMAC, vm.TapName, vm.SSHPort, vm.SocketPath, vm.WorkDir, vm.UserData, vm.UserID,
		vm.EnableNetwork, vm.CreatedAt, vm.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create vm: %w", err)
	}
	return nil
}

// GetVM retrieves a VM by ID. Returns nil if not found.
func (d *DB) GetVM(id string) (*VM, error) {
	return scanVMRow(d.db.QueryRow(
		`SELECT `+vmColumns+` FROM vms WHERE id = $1`, id,
	))
}

// GetVMBySSHPort retrieves a VM by its forwarded SSH host port.
// Returns nil if no VM is assigned to the port.
func (d *DB) GetVMBySSHPort(port int) (*VM, error) {
	return scanVMRow(d.db.QueryRow(
		`SELECT `+vmColumns+` FROM vms WHERE ssh_port = $1`, port,
	))
}

// ListVMs returns all VM records.
func (d *DB) ListVMs() ([]VM, error) {
	rows, err := d.db.Query(
		`SELECT ` + vmColumns + ` FROM vms ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}
	defer rows.Close()

	var vms []VM
	for rows.Next() {
		vm, err := scanVMRows(rows)
		if err != nil {
			return nil, err
		}
		vms = append(vms, *vm)
	}
	return vms, rows.Err()
}

// ListVMsByUser returns VM records owned by a specific user.
func (d *DB) ListVMsByUser(userID string) ([]VM, error) {
	rows, err := d.db.Query(
		`SELECT `+vmColumns+` FROM vms WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list vms by user: %w", err)
	}
	defer rows.Close()

	var vms []VM
	for rows.Next() {
		vm, err := scanVMRows(rows)
		if err != nil {
			return nil, err
		}
		vms = append(vms, *vm)
	}
	return vms, rows.Err()
}

// CountVMsByUser returns the number of VMs owned by a user.
func (d *DB) CountVMsByUser(userID string) (int, error) {
	var count int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM vms WHERE user_id = $1`, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count vms by user: %w", err)
	}
	return count, nil
}

// UpdateVMState sets the state and updated_at for a VM.
func (d *DB) UpdateVMState(id, state string) error {
	_, err := d.db.Exec(
		`UPDATE vms SET state = $1, updated_at = $2 WHERE id = $3`,
		state, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update vm state: %w", err)
	}
	return nil
}

// UpdateVMNetwork updates the network fields after setup.
func (d *DB) UpdateVMNetwork(id, tapName, guestIP, guestMAC string) error {
	_, err := d.db.Exec(
		`UPDATE vms SET tap_name = $1, guest_ip = $2, guest_mac = $3, updated_at = $4 WHERE id = $5`,
		tapName, guestIP, guestMAC, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update vm network: %w", err)
	}
	return nil
}

// DeleteVM removes a VM record by ID.
func (d *DB) DeleteVM(id string) error {
	_, err := d.db.Exec(`DELETE FROM vms WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete vm: %w", err)
	}
	return nil
}

// ListVMsByState returns VMs in the given state.
func (d *DB) ListVMsByState(state string) ([]VM, error) {
	rows, err := d.db.Query(
		`SELECT `+vmColumns+` FROM vms WHERE state = $1 ORDER BY created_at DESC`, state,
	)
	if err != nil {
		return nil, fmt.Errorf("list vms by state: %w", err)
	}
	defer rows.Close()

	var vms []VM
	for rows.Next() {
		vm, err := scanVMRows(rows)
		if err != nil {
			return nil, err
		}
		vms = append(vms, *vm)
	}
	return vms, rows.Err()
}

// MarkStaleVMs sets any "starting" or "running" VMs to "error" state.
// Called at startup to recover from unclean shutdowns.
func (d *DB) MarkStaleVMs() (int64, error) {
	res, err := d.db.Exec(
		`UPDATE vms SET state = $1, updated_at = $2 WHERE state IN ($3, $4, $5)`,
		VMStateError, time.Now().UTC(),
		VMStateStarting, VMStateRunning, VMStateStopping,
	)
	if err != nil {
		return 0, fmt.Errorf("mark stale vms: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// --- helpers ---

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVMRow(row *sql.Row) (*VM, error) {
	vm := &VM{}
	err := row.Scan(
		&vm.ID, &vm.ImageID, &vm.TemplateID, &vm.State, &vm.VCPUCount, &vm.MemMib,
		&vm.GuestIP, &vm.GuestMAC, &vm.TapName, &vm.SSHPort, &vm.SocketPath, &vm.WorkDir,
		&vm.UserData, &vm.UserID, &vm.EnableNetwork, &vm.CreatedAt, &vm.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan vm: %w", err)
	}
	return vm, nil
}

func scanVMRows(row rowScanner) (*VM, error) {
	vm := &VM{}
	err := row.Scan(
		&vm.ID, &vm.ImageID, &vm.TemplateID, &vm.State, &vm.VCPUCount, &vm.MemMib,
		&vm.GuestIP, &vm.GuestMAC, &vm.TapName, &vm.SSHPort, &vm.SocketPath, &vm.WorkDir,
		&vm.UserData, &vm.UserID, &vm.EnableNetwork, &vm.CreatedAt, &vm.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan vm row: %w", err)
	}
	return vm, nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
