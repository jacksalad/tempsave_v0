---
name: tempsave-cli
description: >
  This skill should be used when the user needs to interact with a TempSave file
  storage service. It enables file upload, download, listing, deletion, and folder
  management operations through HTTP API calls. Use this skill when the user asks
  to upload files to, download files from, list files on, delete files from, or
  manage folders on a TempSave server.
license: MIT
metadata:
  category: file-management
  source:
    repository: 'https://github.com/jacksalad/tempsave_v0'
---

# TempSave CLI Skill

This skill provides the ability to interact with a TempSave file storage service through HTTP API calls. The service supports folders — all operations accept a `dir` parameter to target a specific subdirectory.

## Configuration

**IMPORTANT: Before using this skill for the first time, the server URL must be configured.**

Default URL: `http://localhost:8088`

If the user hasn't configured the URL yet, ask them for the actual server URL and update the `TEMPSAVE_SERVER_URL` variable below.

```yaml
# TEMPSAVE_SERVER_URL: http://localhost:8088
```

To change the URL, edit this file and replace the URL above with the actual server address. The `.claude` directory may not be tracked in git; if the file doesn't exist, create it.

## Pre-flight Check

**Before performing any file operations, always verify the service is available:**

```bash
curl -s -o /dev/null -w "%{http_code}" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/files
```

Expected result: HTTP status code `200`

If the result is not `200`, the service may be unavailable. Report this to the user and ask if they want to:
1. Check if the server is running
2. Update the server URL
3. Proceed anyway

## API Operations

### 1. Upload File

Upload a file to the TempSave server. Supports an optional `dir` parameter for subdirectory upload:

```bash
# Upload to root
curl -X POST -F "file=@/path/to/file.txt" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/upload-file

# Upload to a subdirectory (dir is relative to uploads root)
curl -X POST -F "file=@file.txt" -F "dir=myfolder" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/upload-file

# Upload to nested subdirectory
curl -X POST -F "file=@file.txt" -F "dir=myfolder/subfolder" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/upload-file
```

**Success Response:**
```json
{
  "ok": true,
  "message": "上传成功",
  "fileName": "file.txt",
  "size": "1.50 MB",
  "bytes": 1572864
}
```

**Failure Response (insufficient space):**
```json
{
  "ok": false,
  "message": "空间不足！需要 500.00 MB，剩余 200.00 MB",
  "requiredSize": 524288000,
  "availableSize": 209715200
}
```

### 2. Get File List

Get the list of files and folders on the server. Supports `dir` and `limit` parameters:

```bash
# Get default list from root (max 5 files)
curl ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/files

# Get all files and folders from root
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/files?limit=all"

# Get all files and folders in a subdirectory
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/files?limit=all&dir=myfolder"

# Get all files in nested subdirectory
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/files?limit=all&dir=myfolder/subfolder"
```

**Response Example** — entries now include a `type` field (`"file"` or `"dir"`):
```json
[
  {
    "name": "myfolder",
    "time": "2026-04-27 3:45PM",
    "size": "",
    "type": "dir"
  },
  {
    "name": "document.pdf",
    "time": "2026-03-22 3:45PM",
    "size": "2.35 MB",
    "type": "file"
  }
]
```

Directories are listed first (sorted by name), then files (sorted by modification time descending). Directory entries have an empty `size` field.

### 3. Download File

Download a file from the server. For files in subdirectories, include the directory path in the URL:

```bash
# Download from root
curl -O ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/uploads/file.txt

# Download from subdirectory
curl -O ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/uploads/myfolder/file.txt

# Download from nested subdirectory
curl -o my_file.txt ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/uploads/myfolder/subfolder/file.txt
```

### 4. Delete File

Delete a file from the server. Accepts `name` and optional `dir` form parameters (POST only):

```bash
# Delete from root
curl -X POST -d "name=file.txt" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/del

# Delete from subdirectory
curl -X POST -d "name=file.txt&dir=myfolder" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/del
```

**Note:** The `/del` endpoint uses POST, not GET. For filenames with special characters or Chinese, use `--data-urlencode`:

```bash
curl -X POST -G "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/del" --data-urlencode "name=文件 名.txt" --data-urlencode "dir=myfolder"
```

**Response:**
```
file.txt deleted.
```

### 5. Check Storage Space

Check if there's enough space before uploading:

```bash
# Check for root upload
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/check-space?size=1048576&name=test.txt"

# Check for subdirectory upload
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/check-space?size=1048576&name=test.txt&dir=myfolder"
```

**Parameters:**
- `size` — File size in bytes
- `name` — Filename (to check if overwriting an existing file)
- `dir` — Target subdirectory (optional)

**Space Available Response:**
```json
{
  "ok": true,
  "message": "空间充足",
  "currentSize": 524288000,
  "availableSize": 524288000,
  "maxSize": 1073741824
}
```

### 6. Create Folder

Create one or more directories. Auto-creates parent directories if they don't exist:

```bash
# Create a single folder
curl -X POST -d "dir=newfolder" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/mkdir

# Create nested folders
curl -X POST -d "dir=parent/child/grandchild" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/mkdir
```

**Success Response:**
```json
{
  "ok": true,
  "message": "目录创建成功",
  "dir": "newfolder"
}
```

### 7. Delete Folder

Remove an empty directory. The directory must be empty (no files or subdirectories):

```bash
curl -X POST -d "dir=emptyfolder" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/rmdir
```

**Success Response:**
```json
{
  "ok": true,
  "message": "目录已删除"
}
```

**Failure Response (not empty):**
```json
{
  "ok": false,
  "message": "删除目录失败: ...（目录可能不为空）"
}
```

## Workflow Guidelines

### When uploading files:
1. Check service availability with `/files` endpoint
2. If the target directory doesn't exist, create it with `/mkdir`
3. Optionally check storage space with `/check-space` if file size is known
4. Execute upload command (include `dir` parameter for subdirectory uploads)
5. Verify the response indicates success (`"ok": true`)
6. Report the result to the user

### When downloading files:
1. Check service availability
2. Optionally list files to confirm the file exists (include `dir` if in subdirectory)
3. Construct the download URL including directory path if needed
4. Execute download command
5. Verify the file was downloaded successfully
6. Report the result to the user

### When listing files:
1. Check service availability
2. Ask or determine which directory to list (root by default)
3. Execute the list command with `dir` parameter
4. Parse and present the file list to the user, noting which entries are files vs directories

### When deleting files:
1. Check service availability
2. Confirm with user before deletion (optional but recommended)
3. Execute delete command with `name` and optional `dir` parameters
4. Verify the response indicates success
5. Report the result to the user

### When managing folders:
1. Check service availability
2. For creation: determine the folder path, use `/mkdir`
3. For deletion: confirm the folder is empty first (list its contents), then use `/rmdir`
4. Verify the response and report results

## Error Handling

| HTTP Status Code | Description |
|-----------------|-------------|
| 200 | Success |
| 400 | Bad request (invalid parameters, path traversal detected) |
| 404 | Resource not found (directory doesn't exist) |
| 405 | Method not allowed |
| 500 | Internal server error |
| 507 | Insufficient storage |

## Storage Limits

- Maximum total capacity: **1 GB** (recursively counted across all subdirectories)
- Single file size: No limit (subject to total capacity)
- Memory buffer: 32 MB (large files use streaming)

## Important Notes

1. **File Overwrite**: Uploading a file with the same name to the same directory will overwrite the existing file
2. **Folder Deletion**: Only empty folders can be deleted; delete all files inside first
3. **Path Security**: The `dir` parameter rejects `..` (path traversal) and absolute paths
4. **URL Encoding**: Filenames with special characters require URL encoding
5. **Network Timeout**: For large files, consider adjusting curl timeout:
   ```bash
   curl --max-time 300 -X POST -F "file=@large_file.zip" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/upload-file
   ```
6. **Spaces in Paths**: Use `--data-urlencode` for paths containing spaces when using curl

## URL Configuration Instructions

To update the server URL:

1. Locate this SKILL.md file in `.claude/skills/tempsave-cli/SKILL.md`
2. Find the line: `# TEMPSAVE_SERVER_URL: http://localhost:8088`
3. Replace `http://localhost:8088` with the actual server address
4. Save the file

The URL change will take effect immediately for subsequent operations.
