// Lab 8: Implement a network video content service (server)

package storage

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	pb "tritontube/internal/proto"
)

// this is basically only the GRPC server which stores at <dir>/<port> -- the folder is created when a new server is created
// Implement a network video content service (server)
type StorageService struct {
	pb.UnimplementedStorageServiceServer
	baseDir string
}

func NewStorageService(baseDir string) (*StorageService, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory %s: %w", baseDir, err)
	}
	return &StorageService{baseDir: baseDir}, nil
}

func (ss *StorageService) Read(ctx context.Context, rr *pb.ReadRequest) (*pb.ReadResponse, error) {
	// fmt.Printf("base directory %s\n", ss.baseDir)
	filePath := filepath.Join(ss.baseDir, rr.VideoId, rr.FileName)
	fmt.Printf("Read request received for %s\n", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("file not found %s\n", filePath)
			return nil, fmt.Errorf("file not found: %w", err)
		}
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return &pb.ReadResponse{
		FileData: data,
	}, nil
}

func (ss *StorageService) Remove(ctx context.Context, rr *pb.RemoveRequest) (*pb.RemoveResponse, error) {
	filePath := filepath.Join(ss.baseDir, rr.VideoId, rr.FileName)
	fmt.Printf("Remove request received for %s\n", filePath)
	err := os.Remove(filePath)
	if err != nil {
		fmt.Printf("Failed to remove file: %v\n", err)
		return nil, err
	} else {
		fmt.Println("File removed successfully.")
		return &pb.RemoveResponse{}, nil
	}
}

func (ss *StorageService) Write(ctx context.Context, wr *pb.WriteRequest) (*pb.WriteResponse, error) {
	videoDir := filepath.Join(ss.baseDir, wr.VideoId)

	if err := os.MkdirAll(videoDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create video directory %s: %w", videoDir, err)
	}

	filePath := filepath.Join(videoDir, wr.FileName)
	fmt.Printf("Write request received for %s\n", filePath)
	err := os.WriteFile(filePath, wr.FileData, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return &pb.WriteResponse{}, nil

}

func (ss *StorageService) List(ctx context.Context, lr *pb.ListRequest) (*pb.ListResponse, error) {
	responseList := make([]string, 0)

	entries, err := os.ReadDir(ss.baseDir)
	if err != nil {
		// log.Fatal(err)
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subdir := entry.Name()
			subdirPath := filepath.Join(ss.baseDir, subdir)

			subEntries, err := os.ReadDir(subdirPath)
			if err != nil {
				log.Printf("Skipping %s: %v", subdirPath, err)
				continue
			}

			for _, subEntry := range subEntries {
				if !subEntry.IsDir() {
					responseList = append(responseList, path.Join(subdir, subEntry.Name()))
				}
			}
		}
	}

	return &pb.ListResponse{
		Files: responseList,
	}, nil

}

// go run ./cmd/storage -host localhost -port 8090 "./storage/8090"
