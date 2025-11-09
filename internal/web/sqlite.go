// Lab 7: Implement a SQLite video metadata service

package web

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteVideoMetadataService struct {
	db *sql.DB
}

// Uncomment the following line to ensure SQLiteVideoMetadataService implements VideoMetadataService
var _ VideoMetadataService = (*SQLiteVideoMetadataService)(nil)

func NewSQLiteVideoMetadataService(dbPath string) (*SQLiteVideoMetadataService, error) {
	_, err := os.Stat(dbPath)
	dbExists := !errors.Is(err, os.ErrNotExist)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if !dbExists {
		createTableSQL := `
			CREATE TABLE videos (
				id TEXT primary key,
				uploaded_at DATETIME NOT NULL
			);`

		_, err := db.Exec(createTableSQL)

		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create videos table: %w", err)
		}
	}

	return &SQLiteVideoMetadataService{db: db}, nil
}

func (s *SQLiteVideoMetadataService) Read(id string) (*VideoMetadata, error) {
	query := "SELECT id, uploaded_at FROM videos WHERE id = ?"
	row := s.db.QueryRow(query, id)

	video := &VideoMetadata{}
	var uploadedAtStr string

	err := row.Scan(&video.Id, &uploadedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query video: %w", err)
	}

	video.UploadedAt, err = time.Parse(time.RFC3339, uploadedAtStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uploaded_at time: %w", err)
	}

	return video, nil
}

func (s *SQLiteVideoMetadataService) List() ([]VideoMetadata, error) {
	query := "SELECT id, uploaded_at FROM videos ORDER BY uploaded_at DESC"
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query videos: %w", err)
	}
	defer rows.Close()

	var videos []VideoMetadata
	for rows.Next() {
		video := VideoMetadata{}
		var uploadedAtStr string

		err := rows.Scan(&video.Id, &uploadedAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan video row: %w", err)
		}

		// Parse the time string
		video.UploadedAt, err = time.Parse(time.RFC3339, uploadedAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse uploaded_at time for video %s: %w", video.Id, err)
		}
		videos = append(videos, video)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating video rows: %w", err)
	}

	return videos, nil
}

func (s *SQLiteVideoMetadataService) Create(videoId string, uploadedAt time.Time) error {
	query := "INSERT INTO videos (id, uploaded_at) VALUES (?, ?)"
	uploadedAtStr := uploadedAt.Format(time.RFC3339)

	_, err := s.db.Exec(query, videoId, uploadedAtStr)
	if err != nil {
		return fmt.Errorf("failed to insert video: %w", err)
	}
	return nil
}
