package db

import (
	"database/sql"
	"fmt"
)

type Image struct {
	ID           int64
	Name         string
	Description  string
	BootType     string
	KernelFile   string
	InitrdFile   string
	Cmdline      string
	IPXEScript   string
	Status       string // ready, downloading, error
	StatusDetail string
	CatalogID    string
	CatalogHash  string
	Icon         string
	IconColor    string
	CreatedAt    string
	UpdatedAt    string
}

const BootTypeLinux = "linux"

const (
	ImageStatusReady       = "ready"
	ImageStatusDownloading = "downloading"
	ImageStatusError       = "error"
)

const imageColumns = `id, name, description, boot_type, kernel_file, initrd_file, cmdline, ipxe_script, status, status_detail, catalog_id, catalog_hash, COALESCE(icon, ''), COALESCE(icon_color, ''), created_at, updated_at`

func scanImage(row interface{ Scan(...any) error }) (*Image, error) {
	var img Image
	err := row.Scan(&img.ID, &img.Name, &img.Description, &img.BootType,
		&img.KernelFile, &img.InitrdFile, &img.Cmdline, &img.IPXEScript,
		&img.Status, &img.StatusDetail, &img.CatalogID, &img.CatalogHash,
		&img.Icon, &img.IconColor,
		&img.CreatedAt, &img.UpdatedAt)
	return &img, err
}

func ListImages(d *sql.DB) ([]Image, error) {
	rows, err := d.Query(`SELECT ` + imageColumns + ` FROM images ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []Image
	for rows.Next() {
		img, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		images = append(images, *img)
	}
	return images, rows.Err()
}

func GetImage(d *sql.DB, id int64) (*Image, error) {
	img, err := scanImage(d.QueryRow(`SELECT `+imageColumns+` FROM images WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}

func GetImageByCatalogID(d *sql.DB, catalogID string) (*Image, error) {
	img, err := scanImage(d.QueryRow(`SELECT `+imageColumns+` FROM images WHERE catalog_id = ?`, catalogID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}

func CreateImage(d *sql.DB, name, description, bootType, kernelFile, initrdFile, cmdline, ipxeScript string) (int64, error) {
	if bootType == "" {
		bootType = BootTypeLinux
	}
	result, err := d.Exec(`INSERT INTO images (name, description, boot_type, kernel_file, initrd_file, cmdline, ipxe_script) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		name, description, bootType, kernelFile, initrdFile, cmdline, ipxeScript)
	if err != nil {
		return 0, fmt.Errorf("insert image: %w", err)
	}
	return result.LastInsertId()
}

func CreateCatalogImage(d *sql.DB, name, description, bootType, cmdline, ipxeScript, catalogID, catalogHash, icon, iconColor string) (int64, error) {
	if bootType == "" {
		bootType = BootTypeLinux
	}
	result, err := d.Exec(
		`INSERT INTO images (name, description, boot_type, kernel_file, initrd_file, cmdline, ipxe_script, status, catalog_id, catalog_hash, icon, icon_color) VALUES (?, ?, ?, '', '', ?, ?, 'downloading', ?, ?, ?, ?)`,
		name, description, bootType, cmdline, ipxeScript, catalogID, catalogHash, icon, iconColor)
	if err != nil {
		return 0, fmt.Errorf("insert catalog image: %w", err)
	}
	return result.LastInsertId()
}

func UpdateImageStatus(d *sql.DB, id int64, status, detail string) error {
	_, err := d.Exec(`UPDATE images SET status = ?, status_detail = ?, updated_at = datetime('now') WHERE id = ?`, status, detail, id)
	return err
}

func UpdateImageFiles(d *sql.DB, id int64, kernelFile string) error {
	_, err := d.Exec(`UPDATE images SET kernel_file = ?, updated_at = datetime('now') WHERE id = ?`, kernelFile, id)
	return err
}

func UpdateImage(d *sql.DB, id int64, name, description, bootType, cmdline, ipxeScript string) error {
	_, err := d.Exec(`UPDATE images SET name = ?, description = ?, boot_type = ?, cmdline = ?, ipxe_script = ?, updated_at = datetime('now') WHERE id = ?`,
		name, description, bootType, cmdline, ipxeScript, id)
	return err
}

func ResetCatalogImage(d *sql.DB, id int64, name, description, bootType, cmdline, ipxeScript, catalogHash, icon, iconColor string) error {
	_, err := d.Exec(`UPDATE images SET name = ?, description = ?, boot_type = ?, cmdline = ?, ipxe_script = ?,
		kernel_file = '', initrd_file = '', status = 'downloading', status_detail = '',
		catalog_hash = ?, icon = ?, icon_color = ?, updated_at = datetime('now') WHERE id = ?`,
		name, description, bootType, cmdline, ipxeScript, catalogHash, icon, iconColor, id)
	return err
}

func UpdateImageIcon(d *sql.DB, id int64, icon, iconColor string) error {
	_, err := d.Exec(`UPDATE images SET icon = ?, icon_color = ?, updated_at = datetime('now') WHERE id = ?`,
		icon, iconColor, id)
	return err
}

func DeleteImage(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM images WHERE id = ?`, id)
	return err
}
