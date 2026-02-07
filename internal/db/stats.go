package db

import "database/sql"

type Stats struct {
	Systems  SystemStats  `json:"systems"`
	Images   ImageStats   `json:"images"`
	Profiles int         `json:"profiles"`
	Webhooks WebhookStats `json:"webhooks"`
}

type SystemStats struct {
	Total        int `json:"total"`
	Discovered   int `json:"discovered"`
	Queued       int `json:"queued"`
	Provisioning int `json:"provisioning"`
	Ready        int `json:"ready"`
	Failed       int `json:"failed"`
}

type ImageStats struct {
	Total       int `json:"total"`
	Ready       int `json:"ready"`
	Downloading int `json:"downloading"`
	Error       int `json:"error"`
}

type WebhookStats struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

func GetStats(d *sql.DB) (*Stats, error) {
	var s Stats

	rows, err := d.Query(`SELECT state, COUNT(*) FROM systems GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return nil, err
		}
		s.Systems.Total += n
		switch state {
		case "discovered":
			s.Systems.Discovered = n
		case "queued":
			s.Systems.Queued = n
		case "provisioning":
			s.Systems.Provisioning = n
		case "ready":
			s.Systems.Ready = n
		case "failed":
			s.Systems.Failed = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows2, err := d.Query(`SELECT status, COUNT(*) FROM images GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var status string
		var n int
		if err := rows2.Scan(&status, &n); err != nil {
			return nil, err
		}
		s.Images.Total += n
		switch status {
		case "ready":
			s.Images.Ready = n
		case "downloading":
			s.Images.Downloading = n
		case "error":
			s.Images.Error = n
		}
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	if err := d.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&s.Profiles); err != nil {
		return nil, err
	}

	if err := d.QueryRow(`SELECT COUNT(*) FROM webhooks`).Scan(&s.Webhooks.Total); err != nil {
		return nil, err
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM webhooks WHERE enabled = 1`).Scan(&s.Webhooks.Enabled); err != nil {
		return nil, err
	}

	return &s, nil
}
