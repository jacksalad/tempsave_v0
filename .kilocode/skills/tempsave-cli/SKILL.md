---
name: tempsave-cli
description: >
  This skill should be used when the user needs to interact with a TempSave file
  storage service. It enables file upload, download, listing, and deletion operations
  through HTTP API calls. Use this skill when the user asks to upload files to,
  download files from, list files on, or delete files from a TempSave server.
license: MIT
metadata:
  category: file-management
  source:
    repository: 'https://github.com/local/tempsave-cli'
    path: tempsave-cli
---

# TempSave CLI Skill

This skill provides the ability to interact with a TempSave file storage service through HTTP API calls.

## Configuration

**IMPORTANT: Before using this skill for the first time, the server URL must be configured.**

Default URL: `http://localhost:8088`

If the user hasn't configured the URL yet, ask them for the actual server URL and update the `TEMPSAVE_SERVER_URL` variable below.

```yaml
# TEMPSAVE_SERVER_URL: http://localhost:8088
```

To change the URL, edit this file and replace the URL above with the actual server address.

## Pre-flight Check

**Before performing any file operations, always verify the service is available:**

Execute the following command to check service availability:

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

Upload a file to the TempSave server:

```bash
curl -X POST -F "file=@/path/to/file.txt" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/upload-file
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

Get the list of files on the server:

```bash
# Get default list (max 5 files)
curl ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/files

# Get all files
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/files?limit=all"
```

**Response Example:**
```json
[
  {
    "name": "document.pdf",
    "time": "2026-03-22 3:45PM",
    "size": "2.35 MB"
  },
  {
    "name": "image.png",
    "time": "2026-03-22 2:30PM",
    "size": "156.78 KB"
  }
]
```

### 3. Download File

Download a file from the server:

```bash
# Download and save with original filename
curl -O ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/uploads/file.txt

# Download and specify save filename
curl -o my_file.txt ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/uploads/file.txt
```

### 4. Delete File

Delete a file from the server:

```bash
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/del?name=file.txt"
```

**Note:** For filenames with special characters or Chinese, use URL encoding:

```bash
curl -G "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/del" --data-urlencode "name=文件 名.txt"
```

**Response:**
```
file.txt deleted.
```

### 5. Check Storage Space

Check if there's enough space before uploading:

```bash
curl "${TEMPSAVE_SERVER_URL:-http://localhost:8088}/check-space?size=1048576&name=test.txt"
```

**Parameters:**
- `size` - File size in bytes
- `name` - Filename (to check if overwriting an existing file)

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

## Workflow Guidelines

### When uploading files:
1. First check service availability with `/files` endpoint
2. Optionally check storage space with `/check-space` if file size is known
3. Execute upload command
4. Verify the response indicates success (`"ok": true`)
5. Report the result to the user

### When downloading files:
1. First check service availability
2. Optionally list files to confirm the file exists
3. Execute download command
4. Verify the file was downloaded successfully
5. Report the result to the user

### When listing files:
1. Check service availability
2. Execute the list command
3. Parse and present the file list to the user in a readable format

### When deleting files:
1. Check service availability
2. Confirm with user before deletion (optional but recommended)
3. Execute delete command
4. Verify the response indicates success
5. Report the result to the user

## Error Handling

| HTTP Status Code | Description |
|-----------------|-------------|
| 200 | Success |
| 400 | Bad request (missing file or invalid file) |
| 405 | Method not allowed |
| 500 | Internal server error |
| 507 | Insufficient storage |

## Storage Limits

- Maximum total capacity: **1 GB**
- Single file size: No limit (subject to total capacity)
- Memory buffer: 32 MB (large files use streaming)

## Important Notes

1. **File Overwrite**: Uploading a file with the same name will automatically overwrite the existing file
2. **URL Encoding**: Filenames with special characters require URL encoding
3. **Network Timeout**: For large files, consider adjusting curl timeout:
   ```bash
   curl --max-time 300 -X POST -F "file=@large_file.zip" ${TEMPSAVE_SERVER_URL:-http://localhost:8088}/upload-file
   ```

## URL Configuration Instructions

To update the server URL:

1. Locate this SKILL.md file in `.kilocode/skills/tempsave-cli/SKILL.md`
2. Find the line: `# TEMPSAVE_SERVER_URL: http://localhost:8088`
3. Replace `http://localhost:8088` with the actual server URL
4. Save the file

The URL change will take effect immediately for subsequent operations.
