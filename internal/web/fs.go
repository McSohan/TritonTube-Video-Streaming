// Lab 7: Implement a local filesystem video content service

package web

import (
	"fmt"
	"os"
	"path/filepath"
)

// FSVideoContentService implements VideoContentService using the local filesystem.
type FSVideoContentService struct {
	baseDir string
}

func NewFSVideoContentService(baseDir string) (*FSVideoContentService, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory %s: %w", baseDir, err)
	}
	return &FSVideoContentService{baseDir: baseDir}, nil
}

func (f *FSVideoContentService) Read(videoId string, filename string) ([]byte, error) {
	filePath := filepath.Join(f.baseDir, videoId, filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %w", err)
		}
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return data, nil
}

func (f *FSVideoContentService) Write(videoId string, filename string, data []byte) error {
	videoDir := filepath.Join(f.baseDir, videoId)
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		return fmt.Errorf("failed to create video directory %s: %w", videoDir, err)
	}

	filePath := filepath.Join(videoDir, filename)
	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// Uncomment the following line to ensure FSVideoContentService implements VideoContentService
var _ VideoContentService = (*FSVideoContentService)(nil)
