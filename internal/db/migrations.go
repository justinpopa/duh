package db

import (
	"database/sql"
	"fmt"
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS images (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		kernel_file TEXT NOT NULL,
		initrd_file TEXT NOT NULL,
		cmdline     TEXT NOT NULL DEFAULT '',
		created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS systems (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		mac        TEXT NOT NULL UNIQUE,
		hostname   TEXT NOT NULL DEFAULT '',
		reimage    INTEGER NOT NULL DEFAULT 0,
		image_id   INTEGER REFERENCES images(id) ON DELETE SET NULL,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);`,

	`ALTER TABLE systems ADD COLUMN ip_addr TEXT NOT NULL DEFAULT '';
	 ALTER TABLE systems ADD COLUMN last_seen_at DATETIME;`,

	`ALTER TABLE images ADD COLUMN boot_type TEXT NOT NULL DEFAULT 'linux';
	 ALTER TABLE images ADD COLUMN ipxe_script TEXT NOT NULL DEFAULT '';`,

	`ALTER TABLE images ADD COLUMN status TEXT NOT NULL DEFAULT 'ready';
	 ALTER TABLE images ADD COLUMN status_detail TEXT NOT NULL DEFAULT '';
	 ALTER TABLE images ADD COLUMN catalog_id TEXT NOT NULL DEFAULT '';`,

	`CREATE TABLE IF NOT EXISTS profiles (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		name            TEXT NOT NULL,
		description     TEXT NOT NULL DEFAULT '',
		os_family       TEXT NOT NULL DEFAULT 'custom',
		config_template TEXT NOT NULL DEFAULT '',
		kernel_params   TEXT NOT NULL DEFAULT '',
		default_vars    TEXT NOT NULL DEFAULT '{}',
		created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
	);

	ALTER TABLE systems ADD COLUMN profile_id INTEGER REFERENCES profiles(id) ON DELETE SET NULL;
	ALTER TABLE systems ADD COLUMN vars TEXT NOT NULL DEFAULT '{}';`,

	`ALTER TABLE profiles ADD COLUMN overlay_file TEXT NOT NULL DEFAULT '';`,

	`ALTER TABLE images ADD COLUMN catalog_hash TEXT NOT NULL DEFAULT '';`,

	`ALTER TABLE systems ADD COLUMN confirm_reimage INTEGER NOT NULL DEFAULT 1;`,

	`CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	INSERT OR IGNORE INTO settings (key, value) VALUES ('confirm_reimage', '1');
	ALTER TABLE systems ADD COLUMN confirm_override INTEGER;`,

	`ALTER TABLE profiles ADD COLUMN var_schema TEXT NOT NULL DEFAULT '';
	 ALTER TABLE profiles ADD COLUMN catalog_id TEXT NOT NULL DEFAULT '';`,

	`ALTER TABLE systems ADD COLUMN state TEXT NOT NULL DEFAULT 'discovered';
	 ALTER TABLE systems ADD COLUMN state_changed_at DATETIME;

	 UPDATE systems SET state = 'ready' WHERE reimage = 0 AND hostname != '';
	 UPDATE systems SET state = 'queued' WHERE reimage = 1;
	 UPDATE systems SET state = 'discovered' WHERE hostname = '';
	 UPDATE systems SET state_changed_at = updated_at;

	 CREATE TABLE IF NOT EXISTS webhooks (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		url        TEXT NOT NULL,
		secret     TEXT NOT NULL DEFAULT '',
		events     TEXT NOT NULL DEFAULT '*',
		enabled    INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	 );`,

	`ALTER TABLE images ADD COLUMN icon TEXT NOT NULL DEFAULT '';
	 ALTER TABLE images ADD COLUMN icon_color TEXT NOT NULL DEFAULT '';`,
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&current); err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", i+1); err != nil {
			tx.Rollback()
			return fmt.Errorf("update schema version %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}

	return nil
}
