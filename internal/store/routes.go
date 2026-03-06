package store

import (
	"database/sql"
	"fmt"
	"time"
)

// ProxyRoute maps a subdomain to a VM's internal service port.
type ProxyRoute struct {
	ID         string    `json:"id"`
	VMID       string    `json:"vm_id"`
	Subdomain  string    `json:"subdomain"`
	TargetPort int       `json:"target_port"`
	AutoWake   bool      `json:"auto_wake"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateRoute inserts a new proxy route.
func (d *DB) CreateRoute(r ProxyRoute) error {
	_, err := d.db.Exec(
		`INSERT INTO proxy_routes (id, vm_id, subdomain, target_port, auto_wake, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.ID, r.VMID, r.Subdomain, r.TargetPort, r.AutoWake, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create route: %w", err)
	}
	return nil
}

// GetRoute retrieves a route by ID. Returns nil if not found.
func (d *DB) GetRoute(id string) (*ProxyRoute, error) {
	row := d.db.QueryRow(
		`SELECT id, vm_id, subdomain, target_port, auto_wake, created_at, updated_at
		 FROM proxy_routes WHERE id = $1`, id,
	)
	return scanRoute(row)
}

// GetRouteBySubdomain looks up the route for a given subdomain. Returns nil if not found.
func (d *DB) GetRouteBySubdomain(subdomain string) (*ProxyRoute, error) {
	row := d.db.QueryRow(
		`SELECT id, vm_id, subdomain, target_port, auto_wake, created_at, updated_at
		 FROM proxy_routes WHERE subdomain = $1`, subdomain,
	)
	return scanRoute(row)
}

// ListRoutesByVM returns all proxy routes for a VM.
func (d *DB) ListRoutesByVM(vmID string) ([]ProxyRoute, error) {
	rows, err := d.db.Query(
		`SELECT id, vm_id, subdomain, target_port, auto_wake, created_at, updated_at
		 FROM proxy_routes WHERE vm_id = $1 ORDER BY created_at DESC`, vmID,
	)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	routes := make([]ProxyRoute, 0)
	for rows.Next() {
		var r ProxyRoute
		if err := rows.Scan(&r.ID, &r.VMID, &r.Subdomain, &r.TargetPort, &r.AutoWake,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, r)
	}
	return routes, rows.Err()
}

// DeleteRoute removes a proxy route by ID. Returns true if deleted.
func (d *DB) DeleteRoute(id string) (bool, error) {
	res, err := d.db.Exec(`DELETE FROM proxy_routes WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete route: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteRoutesByVM removes all proxy routes for a VM.
func (d *DB) DeleteRoutesByVM(vmID string) error {
	_, err := d.db.Exec(`DELETE FROM proxy_routes WHERE vm_id = $1`, vmID)
	if err != nil {
		return fmt.Errorf("delete routes for vm: %w", err)
	}
	return nil
}

// ListAllRoutes returns all proxy routes (used by proxy server at startup).
func (d *DB) ListAllRoutes() ([]ProxyRoute, error) {
	rows, err := d.db.Query(
		`SELECT id, vm_id, subdomain, target_port, auto_wake, created_at, updated_at
		 FROM proxy_routes ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all routes: %w", err)
	}
	defer rows.Close()

	routes := make([]ProxyRoute, 0)
	for rows.Next() {
		var r ProxyRoute
		if err := rows.Scan(&r.ID, &r.VMID, &r.Subdomain, &r.TargetPort, &r.AutoWake,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, r)
	}
	return routes, rows.Err()
}

func scanRoute(row *sql.Row) (*ProxyRoute, error) {
	r := &ProxyRoute{}
	err := row.Scan(&r.ID, &r.VMID, &r.Subdomain, &r.TargetPort, &r.AutoWake,
		&r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan route: %w", err)
	}
	return r, nil
}
