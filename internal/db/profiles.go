package db

import (
	"database/sql"
	"fmt"
)

type Profile struct {
	ID             int64
	Name           string
	Description    string
	OSFamily       string
	ConfigTemplate string
	KernelParams   string
	DefaultVars    string
	OverlayFile    string
	VarSchema      string
	CatalogID      string
	CreatedAt      string
	UpdatedAt      string
}

const profileColumns = `id, name, description, os_family, config_template, kernel_params, default_vars, overlay_file, var_schema, catalog_id, created_at, updated_at`

func scanProfile(row interface{ Scan(...any) error }) (*Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.OSFamily,
		&p.ConfigTemplate, &p.KernelParams, &p.DefaultVars, &p.OverlayFile,
		&p.VarSchema, &p.CatalogID,
		&p.CreatedAt, &p.UpdatedAt)
	return &p, err
}

func ListProfiles(d *sql.DB) ([]Profile, error) {
	rows, err := d.Query(`SELECT ` + profileColumns + ` FROM profiles ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}
	return profiles, rows.Err()
}

func GetProfile(d *sql.DB, id int64) (*Profile, error) {
	p, err := scanProfile(d.QueryRow(`SELECT `+profileColumns+` FROM profiles WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func GetProfileByCatalogID(d *sql.DB, catalogID string) (*Profile, error) {
	p, err := scanProfile(d.QueryRow(`SELECT `+profileColumns+` FROM profiles WHERE catalog_id = ?`, catalogID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func CreateProfile(d *sql.DB, name, description, osFamily, configTemplate, kernelParams, defaultVars, overlayFile, varSchema, catalogID string) (int64, error) {
	if osFamily == "" {
		osFamily = "custom"
	}
	if defaultVars == "" {
		defaultVars = "{}"
	}
	result, err := d.Exec(`INSERT INTO profiles (name, description, os_family, config_template, kernel_params, default_vars, overlay_file, var_schema, catalog_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, description, osFamily, configTemplate, kernelParams, defaultVars, overlayFile, varSchema, catalogID)
	if err != nil {
		return 0, fmt.Errorf("insert profile: %w", err)
	}
	return result.LastInsertId()
}

func UpdateProfile(d *sql.DB, id int64, name, description, osFamily, configTemplate, kernelParams, defaultVars, overlayFile, varSchema string) error {
	if osFamily == "" {
		osFamily = "custom"
	}
	if defaultVars == "" {
		defaultVars = "{}"
	}
	_, err := d.Exec(`UPDATE profiles SET name = ?, description = ?, os_family = ?, config_template = ?, kernel_params = ?, default_vars = ?, overlay_file = ?, var_schema = ?, updated_at = datetime('now') WHERE id = ?`,
		name, description, osFamily, configTemplate, kernelParams, defaultVars, overlayFile, varSchema, id)
	return err
}

func DeleteProfile(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	return err
}
