package db

import (
	"database/sql"
)

func GetSetting(d *sql.DB, key string) (string, error) {
	var value string
	err := d.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func SetSetting(d *sql.DB, key, value string) error {
	_, err := d.Exec("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}

func DeleteSetting(d *sql.DB, key string) error {
	_, err := d.Exec("DELETE FROM settings WHERE key = ?", key)
	return err
}
