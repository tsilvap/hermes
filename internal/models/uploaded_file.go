package models

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"
)

type UploadedFile struct {
	ID       int
	Title    string
	Uploader string
	FilePath string
	Created  time.Time
}

func (f *UploadedFile) MIMEType() string {
	return mime.TypeByExtension(filepath.Ext(f.FilePath))
}

func (f *UploadedFile) Type() string {
	return strings.Split(f.MIMEType(), "/")[0]
}

func (f *UploadedFile) FileHref() string {
	if f.Type() == "text" {
		return fmt.Sprintf("/t/%d", f.ID)
	} else {
		return fmt.Sprintf("/u/%d", f.ID)
	}
}

func (f *UploadedFile) RawFileHref() string {
	return fmt.Sprintf("/dl/%d", f.ID)
}

type UploadedFileModel struct {
	DB *sql.DB
}

// Insert a new uploaded file.
func (m *UploadedFileModel) Insert(title, uploader, path string) (int, error) {
	if m.DB == nil {
		return 0, nil
	}

	stmt, err := m.DB.Prepare(`INSERT INTO uploaded_files(title, uploader, file_path, created_at) VALUES(?, ?, ?, datetime('now'))`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	result, err := stmt.Exec(title, uploader, path)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// Get uploaded file by ID.
func (m *UploadedFileModel) Get(id int) (*UploadedFile, error) {
	if m.DB == nil {
		return nil, nil
	}

	f := &UploadedFile{}
	err := m.DB.QueryRow(`SELECT id, title, uploader, file_path, created_at FROM uploaded_files WHERE id = ?`, id).Scan(&f.ID, &f.Title, &f.Uploader, &f.FilePath, &f.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		} else {
			return nil, err
		}
	}
	return f, nil
}

// Return the 10 latest uploaded files.
func (m *UploadedFileModel) Latest() ([]*UploadedFile, error) {
	if m.DB == nil {
		return nil, nil
	}

	rows, err := m.DB.Query(`SELECT id, title, uploader, file_path, created_at FROM uploaded_files ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	latest := []*UploadedFile{}
	for rows.Next() {
		f := &UploadedFile{}
		if err := rows.Scan(&f.ID, &f.Title, &f.Uploader, &f.FilePath, &f.Created); err != nil {
			return nil, err
		}
		latest = append(latest, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return latest, nil
}
