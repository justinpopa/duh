package db

import "database/sql"

type Webhook struct {
	ID        int64
	URL       string
	Secret    string
	Events    string
	Enabled   bool
	CreatedAt string
	UpdatedAt string
}

func ListWebhooks(d *sql.DB) ([]Webhook, error) {
	rows, err := d.Query(`SELECT id, url, secret, events, enabled, created_at, updated_at FROM webhooks ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		webhooks = append(webhooks, w)
	}
	return webhooks, rows.Err()
}

func GetWebhook(d *sql.DB, id int64) (*Webhook, error) {
	var w Webhook
	err := d.QueryRow(`SELECT id, url, secret, events, enabled, created_at, updated_at FROM webhooks WHERE id = ?`, id).
		Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func CreateWebhook(d *sql.DB, url, secret, events string) (int64, error) {
	result, err := d.Exec(`INSERT INTO webhooks (url, secret, events) VALUES (?, ?, ?)`, url, secret, events)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func UpdateWebhook(d *sql.DB, id int64, url, secret, events string, enabled bool) error {
	enabledVal := 0
	if enabled {
		enabledVal = 1
	}
	_, err := d.Exec(`UPDATE webhooks SET url = ?, secret = ?, events = ?, enabled = ?, updated_at = datetime('now') WHERE id = ?`,
		url, secret, events, enabledVal, id)
	return err
}

func DeleteWebhook(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM webhooks WHERE id = ?`, id)
	return err
}

func ListEnabledWebhooks(d *sql.DB) ([]Webhook, error) {
	rows, err := d.Query(`SELECT id, url, secret, events, enabled, created_at, updated_at FROM webhooks WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		webhooks = append(webhooks, w)
	}
	return webhooks, rows.Err()
}
