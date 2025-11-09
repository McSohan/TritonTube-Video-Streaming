// Lab 7: Implement a web server

package web

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type server struct {
	Addr string
	Port int

	metadataService VideoMetadataService
	contentService  VideoContentService

	mux *http.ServeMux
}

func NewServer(
	metadataService VideoMetadataService,
	contentService VideoContentService,
) *server {
	return &server{
		metadataService: metadataService,
		contentService:  contentService,
	}
}

func (s *server) Start(lis net.Listener) error {
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/upload", s.handleUpload)
	s.mux.HandleFunc("/videos/", s.handleVideo)
	s.mux.HandleFunc("/content/", s.handleVideoContent)
	s.mux.HandleFunc("/", s.handleIndex)

	return http.Serve(lis, s.mux)
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	videos, err := s.metadataService.List()
	fmt.Println(err)
	if err != nil {
		http.Error(w, "Failed to list videos", http.StatusInternalServerError)
		return
	}

	err = renderIndex(w, videos)
	if err != nil {
		http.Error(w, "Failed to render index", http.StatusInternalServerError)
		return
	}
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	baseFilename := filepath.Base(handler.Filename)
	videoId := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename))

	// Check if the video Id is already in use
	existingVideo, err := s.metadataService.Read(videoId)
	if err != nil {
		http.Error(w, "Failed to check video metadata", http.StatusInternalServerError)
		return
	}
	if existingVideo != nil {
		http.Error(w, "Video ID already exists", http.StatusConflict)
		return
	}

	err = s.metadataService.Create(videoId, time.Now())
	if err != nil {
		http.Error(w, "Failed to add video metadata", http.StatusInternalServerError)
		return
	}

	videoData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// create a temporary directory for video processing
	tempDir, err := os.MkdirTemp("", "tritontube")
	if err != nil {
		http.Error(w, "Failed to create temporary directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir) // clean up the temp directory

	// save the video as `video.mp4` in the temp directory
	videoPath := filepath.Join(tempDir, "video.mp4")
	err = os.WriteFile(videoPath, videoData, 0644)
	if err != nil {
		http.Error(w, "Failed to save video", http.StatusInternalServerError)
		return
	}

	// path to the manifest file
	manifestPath := filepath.ToSlash(filepath.Join(tempDir, "manifest.mpd"))

	// ffmpeg command to create MPEG-DASH files
	cmd := exec.Command("ffmpeg",
		"-i", videoPath, // input file
		"-c:v", "libx264", // video codec
		"-c:a", "aac", // audio codec
		"-bf", "1", // max 1 b-frame
		"-keyint_min", "120", // minimum keyframe interval
		"-g", "120", // keyframe every 120 frames
		"-sc_threshold", "0", // scene change threshold
		"-b:v", "3000k", // video bitrate
		"-b:a", "128k", // audio bitrate
		"-f", "dash", // dash format
		"-use_timeline", "1", // use timeline
		"-use_template", "1", // use template
		"-init_seg_name", "init-$RepresentationID$.m4s", // init segment naming
		"-media_seg_name", "chunk-$RepresentationID$-$Number%05d$.m4s", // media segment naming
		"-seg_duration", "4", // segment duration in seconds
		manifestPath) // output file

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// run the ffmpeg command
	if err := cmd.Run(); err != nil {
		http.Error(w, "Failed to encode video: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove the original video file
	err = os.Remove(videoPath)
	if err != nil {
		http.Error(w, "Failed to remove original video file", http.StatusInternalServerError)
		return
	}

	// Read all files in the tempDir and write them to the content service
	files, err := os.ReadDir(tempDir)
	if err != nil {
		http.Error(w, "Failed to read temporary directory", http.StatusInternalServerError)
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(tempDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			http.Error(w, "Failed to read file: "+file.Name(), http.StatusInternalServerError)
			return
		}

		err = s.contentService.Write(videoId, file.Name(), data)
		if err != nil {
			log.Println("Failed to write file to content service:", err)
			http.Error(w, "Failed to write file to content service: "+file.Name(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) handleVideo(w http.ResponseWriter, r *http.Request) {
	videoId := r.URL.Path[len("/videos/"):]
	video, err := s.metadataService.Read(videoId)

	if err != nil {
		http.Error(w, "Failed to get video metadata", http.StatusInternalServerError)
		return
	}
	if video == nil {
		http.NotFound(w, r)
		return
	}

	err = renderVideo(w, video)
	if err != nil {
		http.Error(w, "Failed to render video", http.StatusInternalServerError)
		return
	}
}

func (s *server) handleVideoContent(w http.ResponseWriter, r *http.Request) {
	// parse /content/<videoId>/<filename>
	videoId := r.URL.Path[len("/content/"):]
	parts := strings.Split(videoId, "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid content path", http.StatusBadRequest)
		return
	}
	videoId = parts[0]
	filename := parts[1]
	// fmt.Println("Trying to read")
	data, err := s.contentService.Read(videoId, filename)
	if err != nil {
		// fmt.Println("Error here")
		http.Error(w, "Failed to get video content", http.StatusInternalServerError)
		return
	}
	if data == nil {
		http.NotFound(w, r)
		return
	}

	// Serve the file with proper headers.
	if strings.HasSuffix(filename, ".mpd") {
		w.Header().Set("Content-Type", "application/dash+xml")
	} else if strings.HasSuffix(filename, ".m4s") {
		w.Header().Set("Content-Type", "video/mp4")
	}

	// Serve full file.
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		log.Printf("Failed to write .mp4 response: %v", err)
	}
}
