# AGENTS.md

This file provides guidance to agents when working with this code in this repository.

## Build/Run Commands
```bash
go run main.go          # 开发运行（端口 8088）
go build -o tempsave.exe # 构建可执行文件
```

## Architecture Overview
- **后端**: 单文件 Go HTTP 服务器 ([`main.go`](main.go))，使用标准库 `net/http`
- **前端**: 原生 HTML + jQuery + Bootstrap ([`src/index.html`](src/index.html), [`src/assets/main.js`](src/assets/main.js))

## Key Implementation Details
- **分片上传**: 前端将文件按 1MB 切片，后端接收后合并（`mergeFile` 函数）
- **切片命名**: `{uploadDir}/{fileName}_{chunkIndex}`
- **线程安全**: 文件合并使用 `sync.Mutex` 防止并发冲突
- **时间格式**: Go 特定格式 `"2006-01-02 3:04PM"`（不可随意更改）
- **上传目录**: `./uploads`（通过 `/uploads/` 访问）
- **日志**: 写入 `app.log`
- **存储限制**: 总容量上限 1GB（`maxTotalSize = 1 << 30`），上传前通过 `/check-space` 接口预检查

## Storage Limit Logic
- 上传前前端调用 `/check-space` 检查空间是否充足
- 检查时考虑覆盖同名文件场景（减去旧文件大小）
- 空间不足时前端弹窗提示，不上传文件
- 后端 `mergeFile` 函数也有二次校验作为兜底

## API Endpoints
| 路由 | 方法 | 功能 |
|------|------|------|
| `/` | GET | 主页 |
| `/files?limit=` | GET | 获取文件列表 JSON（limit 默认 5，设为 all 获取全部） |
| `/upload` | POST | 分片上传（前端专用） |
| `/upload-file` | POST | 单文件上传（CLI 专用） |
| `/del?name=` | GET | 删除文件 |
| `/check-space?size=&name=` | GET | 检查空间是否足够（返回 JSON） |
| `/assets/` | GET | 静态资源服务 |
| `/uploads/` | GET | 上传文件访问 |

## CLI 使用示例 (curl)
```bash
# 上传文件
curl -X POST -F "file=@/path/to/file.txt" http://localhost:8088/upload-file

# 获取文件列表（默认最多 5 个）
curl http://localhost:8088/files

# 获取所有文件列表
curl http://localhost:8088/files?limit=all

# 删除文件
curl "http://localhost:8088/del?name=file.txt"

# 下载文件
curl -O http://localhost:8088/uploads/file.txt

# 检查空间
curl "http://localhost:8088/check-space?size=1048576&name=test.txt"
```
