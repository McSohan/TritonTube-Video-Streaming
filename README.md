# TritonTube – Distributed Video Hosting Platform

**TritonTube** is a simplified, distributed video hosting system inspired by platforms like YouTube.  
It supports video upload, adaptive streaming, distributed storage, and fault-tolerant metadata management across multiple nodes.  
The project is implemented entirely in **Go**, designed to demonstrate core systems concepts including **consistent hashing**, **inter-service communication**, and **real-time video streaming**.

---

## Features

- **Video Upload & Streaming**
  - Users can upload MP4 videos via a web interface.
  - Videos are automatically transcoded to **MPEG-DASH** using FFmpeg for adaptive bitrate playback.
  - Landing page lists all uploaded videos with links to individual playback pages.

- **Distributed Storage**
  - Videos and metadata are distributed across multiple storage nodes using **consistent hashing**.
  - Content retrieval and placement are transparent to the user.
  - Dynamic scaling supported through an **Admin CLI** that adds or removes storage nodes and automatically migrates files.

- **Fault Tolerance**
  - Metadata replication and consistency managed using an **etcd cluster** running the **RAFT consensus protocol**.
  - The system remains operational during node failures and automatically rebalances leadership.

- **Observability**
  - Built-in metrics and structured logging for performance debugging and system introspection.

---

## Architecture Overview

```
                 ┌────────────────────────────────────┐
                 │            Web Server              │
                 │  - HTTP endpoints (/upload, /videos)│
                 │  - gRPC client to Storage Servers   │
                 └────────────────────────────────────┘
                               │
          ┌────────────────────┴────────────────────┐
          ▼                                         ▼
 ┌───────────────────┐                     ┌───────────────────┐
 │  Storage Server   │                     │  Storage Server   │
 │  - Stores DASH    │                     │  - Serves content │
 │    segments       │                     │    by key hash    │
 │  - gRPC service   │                     │                   │
 └───────────────────┘                     └───────────────────┘
          ▲                                         ▲
          └────────────────────┬────────────────────┘
                               │
                     ┌───────────────────┐
                     │  etcd Cluster     │
                     │  - Metadata store │
                     │  - Leader election│
                     └───────────────────┘
```

---

## Implementation Details

- **Language:** Go 1.22  
- **Video Encoding:** FFmpeg (`libx264`, `aac`, MPEG-DASH segmentation)  
- **Storage:** Local filesystem on distributed nodes  
- **Metadata:** SQLite + optional etcd replication  
- **Networking:** HTTP (for clients) + gRPC (for inter-node communication)  
- **Core Components:**
  - `internal/web/` — Web server, content routing, and metadata logic  
  - `internal/storage/` — Storage node logic and gRPC service  
  - `proto/` — Protocol definitions for admin and storage communication  
  - `cmd/web/`, `cmd/storage/`, `cmd/admin/` — Executables  

---

## Running the System

### 1. Start Storage Nodes
```bash
mkdir -p storage/8090 storage/8091 storage/8092

go run ./cmd/storage -port 8090 ./storage/8090 &
go run ./cmd/storage -port 8091 ./storage/8091 &
go run ./cmd/storage -port 8092 ./storage/8092 &
```

### 2. Start Web Server
```bash
go run ./cmd/web/main.go sqlite ./metadata.db nw "localhost:8081,localhost:8090,localhost:8091,localhost:8092"
```

Access the landing page at **http://localhost:8080**

### 3. Manage Cluster
```bash
# List nodes
go run ./cmd/admin list localhost:8081

# Add a new node
go run ./cmd/admin add localhost:8081 localhost:8093

# Remove a node
go run ./cmd/admin remove localhost:8081 localhost:8090
```

---

## Testing & Validation

- Upload a local `.mp4` file through the browser → the system automatically converts it to **MPEG-DASH** (`manifest.mpd` + `.m4s` segments).  
- Verify playback continuity and adaptive bitrate switching via the embedded player.  
- Add/remove nodes dynamically and confirm that files migrate automatically using consistent hashing.  
- Observe fault tolerance behavior when a node or etcd member is terminated — the cluster reconfigures and remains consistent.

---

## Repository Structure

```
cmd/
 ├── web/        # Web server entrypoint
 ├── storage/    # Storage server entrypoint
 └── admin/      # Admin CLI
internal/
 ├── web/        # HTTP + gRPC handlers, hashing, SQLite/etcd services
 ├── storage/    # File service implementation
 └── proto/      # Generated gRPC code
proto/           # .proto definitions
Makefile         # For protobuf compilation
```

---

## Dependencies

- **Go 1.22+**
- **FFmpeg** (for video transcoding)
- **SQLite** (metadata persistence)
- **etcd (optional)** for distributed metadata replication

### Install FFmpeg
```bash
# Linux
sudo apt install ffmpeg

# macOS
brew install ffmpeg
```
