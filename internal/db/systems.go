package db

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

type System struct {
	ID             int64
	MAC            string
	Hostname       string
	ImageID        *int64
	ProfileID      *int64
	Vars           string
	IPAddr         string
	LastSeenAt     string
	State          string
	StateChangedAt string
	CreatedAt      string
	UpdatedAt      string
}

var macSepRe = regexp.MustCompile(`[:\-.]`)

func normalizeMAC(mac string) (string, error) {
	mac = strings.ToLower(strings.TrimSpace(mac))
	hex := macSepRe.ReplaceAllString(mac, "")
	if len(hex) != 12 {
		return "", fmt.Errorf("invalid MAC address: %s", mac)
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "", fmt.Errorf("invalid MAC address: %s", mac)
		}
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		hex[0:2], hex[2:4], hex[4:6], hex[6:8], hex[8:10], hex[10:12]), nil
}

func ListSystems(d *sql.DB) ([]System, error) {
	rows, err := d.Query(`
		SELECT id, mac, hostname, image_id, profile_id, vars,
		       ip_addr, COALESCE(last_seen_at, ''),
		       state, COALESCE(state_changed_at, ''),
		       created_at, updated_at
		FROM systems ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var systems []System
	for rows.Next() {
		var s System
		if err := rows.Scan(&s.ID, &s.MAC, &s.Hostname, &s.ImageID,
			&s.ProfileID, &s.Vars,
			&s.IPAddr, &s.LastSeenAt,
			&s.State, &s.StateChangedAt,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		systems = append(systems, s)
	}
	return systems, rows.Err()
}

func GetSystemByMAC(d *sql.DB, mac string) (*System, error) {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return nil, err
	}
	var s System
	err = d.QueryRow(`
		SELECT id, mac, hostname, image_id, profile_id, vars,
		       ip_addr, COALESCE(last_seen_at, ''),
		       state, COALESCE(state_changed_at, ''),
		       created_at, updated_at
		FROM systems WHERE mac = ?`, mac).Scan(
		&s.ID, &s.MAC, &s.Hostname, &s.ImageID,
		&s.ProfileID, &s.Vars,
		&s.IPAddr, &s.LastSeenAt,
		&s.State, &s.StateChangedAt,
		&s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func CreateSystem(d *sql.DB, mac, hostname string) (*System, error) {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return nil, err
	}
	result, err := d.Exec(`INSERT INTO systems (mac, hostname) VALUES (?, ?)`, mac, hostname)
	if err != nil {
		return nil, fmt.Errorf("insert system: %w", err)
	}
	id, _ := result.LastInsertId()
	return &System{ID: id, MAC: mac, Hostname: hostname}, nil
}

func UpdateSystemImage(d *sql.DB, id int64, imageID *int64) error {
	_, err := d.Exec(`UPDATE systems SET image_id = ?, updated_at = datetime('now') WHERE id = ?`, imageID, id)
	return err
}

func UpdateSystemState(d *sql.DB, id int64, state string) error {
	_, err := d.Exec(`UPDATE systems SET state = ?, state_changed_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`, state, id)
	return err
}

func TransitionSystemStateByMAC(d *sql.DB, mac, expectedState, newState string) error {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return err
	}
	result, err := d.Exec(`UPDATE systems SET state = ?, state_changed_at = datetime('now'), updated_at = datetime('now') WHERE mac = ? AND state = ?`,
		newState, mac, expectedState)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		// Check if already in target state (idempotent)
		var current string
		err := d.QueryRow(`SELECT state FROM systems WHERE mac = ?`, mac).Scan(&current)
		if err != nil {
			return fmt.Errorf("system not found: %s", mac)
		}
		if current == newState {
			return nil // already in target state
		}
		return fmt.Errorf("state transition failed: expected %s, got %s", expectedState, current)
	}
	return nil
}

func TouchSystem(d *sql.DB, mac, ipAddr string) error {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE systems SET ip_addr = ?, last_seen_at = datetime('now'), updated_at = datetime('now') WHERE mac = ?`, ipAddr, mac)
	return err
}

func AutoRegister(d *sql.DB, mac, ipAddr string) (*System, bool, error) {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return nil, false, err
	}
	result, err := d.Exec(`INSERT OR IGNORE INTO systems (mac, ip_addr, last_seen_at) VALUES (?, ?, datetime('now'))`, mac, ipAddr)
	if err != nil {
		return nil, false, fmt.Errorf("auto-register: %w", err)
	}
	n, _ := result.RowsAffected()
	isNew := n > 0
	if !isNew {
		TouchSystem(d, mac, ipAddr)
	}
	sys, err := GetSystemByMAC(d, mac)
	return sys, isNew, err
}

func GetSystemByID(d *sql.DB, id int64) (*System, error) {
	var s System
	err := d.QueryRow(`
		SELECT id, mac, hostname, image_id, profile_id, vars,
		       ip_addr, COALESCE(last_seen_at, ''),
		       state, COALESCE(state_changed_at, ''),
		       created_at, updated_at
		FROM systems WHERE id = ?`, id).Scan(
		&s.ID, &s.MAC, &s.Hostname, &s.ImageID,
		&s.ProfileID, &s.Vars,
		&s.IPAddr, &s.LastSeenAt,
		&s.State, &s.StateChangedAt,
		&s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func UpdateSystemProfile(d *sql.DB, id int64, profileID *int64) error {
	_, err := d.Exec(`UPDATE systems SET profile_id = ?, updated_at = datetime('now') WHERE id = ?`, profileID, id)
	return err
}

func UpdateSystemVars(d *sql.DB, id int64, vars string) error {
	if vars == "" {
		vars = "{}"
	}
	_, err := d.Exec(`UPDATE systems SET vars = ?, updated_at = datetime('now') WHERE id = ?`, vars, id)
	return err
}

func UpdateSystemInfo(d *sql.DB, id int64, mac, hostname string) error {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE systems SET mac = ?, hostname = ?, updated_at = datetime('now') WHERE id = ?`, mac, hostname, id)
	return err
}

func DeleteSystem(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM systems WHERE id = ?`, id)
	return err
}
