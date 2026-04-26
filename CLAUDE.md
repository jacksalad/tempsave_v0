# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build/Run Commands

```bash
go run main.go            # Start dev server on port 8088
go build -o tempsave.exe  # Build server binary
go build -o tempsave-cli.exe ./cmd/tempsave-cli/  # Build CLI binary
```

## Project Overview

Single-binary file sharing service: Go stdlib HTTP backend (one file) + vanilla HTML/jQuery/Bootstrap frontend.

## Architecture

- **`main.go`** — Self-contained Go HTTP server, zero external dependencies. Uses `net/http`, `encoding/json`, `sync`/`sync/atomic`, `io`. Handles chunked upload assembly, file listing with caching, storage limits, and static file serving.
- **`src/index.html`** — Single-page web UI with drag-and-drop, progress bars, file list, breadcrumb navigation.
- **`src/assets/main.js`** — Frontend logic: 1MB chunked uploads, space pre-check, file list refresh, delete via event delegation, folder navigation with URL hash sync. XSS prevention via `.text()` and DOM API (no innerHTML for user data).
- **`cmd/tempsave-cli/main.go`** — Go CLI binary with subcommands: ls, upload, download, del, mkdir, rmdir, df.
- **`src/assets/css/style.css`** — Custom styles (bg overlay, drop zone).

### Key Design Details

- **Chunked uploads**: Frontend slices files into 1MB chunks. Backend merges via `mergeFile()` which runs in a goroutine with rollback cleanup. Chunk naming: `{uploadDir}/{dir}/{fileName}_{chunkIndex}`.
- **Folder support**: All API endpoints accept a `dir` parameter (`""` = root). Directory paths are validated via `validateDir()` — rejects `..`, absolute paths, and path traversal. `filepath.WalkDir` recursively calculates total size across all subdirectories.
- **Thread safety**: File-level locks via `sync.Map` of mutexes prevent concurrent merge conflicts (lock key is `filepath.Join(dir, fileName)`). File list cache uses `sync.RWMutex` with 2s TTL (root only; subdirectories read fresh). Size cache uses `atomic.Int64` with `sync.Once` lazy init.
- **Storage limit**: Hard cap of 1GB (`maxTotalSize = 1 << 30`). Pre-check via `/check-space` before upload; backend also validates during merge.
- **Time format**: Go reference time `"2006-01-02 3:04PM"` — do not change.
- **HTTP server**: Write timeout 120s, idle timeout 120s, no read timeout (required for large uploads).
- **Logging**: Writes to `app.log` in the working directory.

## API Endpoints

| Route | Method | Description |
|-------|--------|-------------|
| `/` | GET | Web UI |
| `/files?limit=&dir=` | GET | File list JSON (default 5, `all` for all; `dir` for subdirectory) |
| `/upload` | POST | Chunked upload (frontend only, supports `dir` field) |
| `/upload-file` | POST | Single-file upload (CLI/curl, supports `dir` field) |
| `/del` | POST | Delete file (form: `name`, `dir`) |
| `/mkdir` | POST | Create directory (form: `dir`) |
| `/rmdir` | POST | Remove empty directory (form: `dir`) |
| `/check-space?size=&name=&dir=` | GET | Pre-check storage space (returns JSON) |
| `/assets/` | GET | Static files |
| `/uploads/` | GET | Serve uploaded files (including subdirectories) |

File list entries include a `type` field: `"file"` or `"dir"`.

## CLI Usage

### Go CLI binary (recommended):
```bash
tempsave-cli ls                      # List root
tempsave-cli ls myfolder             # List subdirectory
tempsave-cli upload file.txt         # Upload to root
tempsave-cli upload file.txt mydir   # Upload to subdirectory
tempsave-cli download file.txt       # Download from root
tempsave-cli download file.txt dir   # Download from subdirectory
tempsave-cli del file.txt            # Delete from root
tempsave-cli del file.txt dir        # Delete from subdirectory
tempsave-cli mkdir newfolder         # Create folder
tempsave-cli rmdir emptyfolder       # Remove empty folder
tempsave-cli df                      # Show storage usage
tempsave-cli --server http://host:port ls  # Custom server
```

### curl:
```bash
# Upload to root
curl -X POST -F "file=@/path/to/file.txt" http://localhost:8088/upload-file
# Upload to subdirectory
curl -X POST -F "file=@file.txt" -F "dir=myfolder" http://localhost:8088/upload-file
# List files in subdirectory
curl "http://localhost:8088/files?limit=all&dir=myfolder"
# Create directory
curl -X POST -d "dir=newfolder" http://localhost:8088/mkdir
# Download
curl -O http://localhost:8088/uploads/myfolder/file.txt
# Delete from subdirectory
curl -X POST -d "name=file.txt&dir=myfolder" http://localhost:8088/del
```

## Runtime State (not in git)

- `uploads/` — Uploaded files directory
- `app.log` — Server log
