package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Template is a reusable VM configuration (image + cloud-init + resource defaults).
type Template struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ImageID     string    `json:"image_id"`
	UserData    string    `json:"user_data,omitempty"`
	VCPUCount   int       `json:"vcpu_count"`
	MemMib      int       `json:"mem_mib"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateTemplate inserts a new template.
func (d *DB) CreateTemplate(t Template) error {
	_, err := d.db.Exec(
		`INSERT INTO templates (id, name, description, image_id, user_data, vcpu_count, mem_mib, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID, t.Name, t.Description, t.ImageID, t.UserData, t.VCPUCount, t.MemMib,
		t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	return nil
}

// GetTemplate retrieves a template by ID. Returns nil if not found.
func (d *DB) GetTemplate(id string) (*Template, error) {
	return scanTemplate(d.db.QueryRow(
		`SELECT id, name, description, image_id, user_data, vcpu_count, mem_mib, created_at, updated_at
		 FROM templates WHERE id = $1`, id,
	))
}

// GetTemplateByName retrieves a template by unique name. Returns nil if not found.
func (d *DB) GetTemplateByName(name string) (*Template, error) {
	return scanTemplate(d.db.QueryRow(
		`SELECT id, name, description, image_id, user_data, vcpu_count, mem_mib, created_at, updated_at
		 FROM templates WHERE name = $1`, name,
	))
}

// ListTemplates returns all templates.
func (d *DB) ListTemplates() ([]Template, error) {
	rows, err := d.db.Query(
		`SELECT id, name, description, image_id, user_data, vcpu_count, mem_mib, created_at, updated_at
		 FROM templates ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	templates := make([]Template, 0)
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ImageID, &t.UserData,
			&t.VCPUCount, &t.MemMib, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// DeleteTemplate removes a template by ID. Returns true if a row was deleted.
func (d *DB) DeleteTemplate(id string) (bool, error) {
	res, err := d.db.Exec(`DELETE FROM templates WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete template: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func scanTemplate(row *sql.Row) (*Template, error) {
	t := &Template{}
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.ImageID, &t.UserData,
		&t.VCPUCount, &t.MemMib, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan template: %w", err)
	}
	return t, nil
}
