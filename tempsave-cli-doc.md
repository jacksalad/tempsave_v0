# TempSave CLI 使用文档

本文档介绍如何通过命令行操作 TempSave 临时网盘服务，支持文件夹管理。

## 前提条件

- 服务已启动（默认端口 8088）
- Go CLI 工具已构建：`go build -o tempsave-cli.exe ./cmd/tempsave-cli/`
- 或使用 curl（需已安装）

## 方法一：Go CLI 工具（推荐）

### 安装

```bash
cd cmd/tempsave-cli
go build -o tempsave-cli.exe
# 或直接构建到项目根目录
go build -o tempsave-cli.exe ./cmd/tempsave-cli/
```

### 基本用法

```bash
# 设置服务器地址（默认 http://localhost:8088）
# 方式1：--server 参数
tempsave-cli --server http://192.168.1.100:8088 ls

# 方式2：环境变量
export TESERVER=http://192.168.1.100:8088
tempsave-cli ls
```

### 命令参考

#### 1. 列出文件/文件夹 `ls [dir]`

```bash
# 列出根目录
tempsave-cli ls

# 列出子目录
tempsave-cli ls myfolder
tempsave-cli ls "myfolder/subfolder"
```

**输出示例：**
```
Type Name                            Time                 Size
--------------------------------------------------------------------------------
DIR  myfolder                        2026-04-27 3:45PM
FILE document.pdf                    2026-04-27 2:30PM     2.35 MB
FILE image.png                       2026-04-27 1:00PM    156.78 KB
```

#### 2. 上传文件 `upload <filepath> [dir]`

```bash
# 上传到根目录
tempsave-cli upload file.txt

# 上传到指定目录（目录不存在会自动创建）
tempsave-cli upload file.txt myfolder
tempsave-cli upload large_file.zip "myfolder/subfolder"
```

**成功响应：**
```json
{
  "ok": true,
  "message": "上传成功",
  "fileName": "file.txt",
  "size": "1.50 MB",
  "bytes": 1572864
}
```

#### 3. 下载文件 `download <name> [dir]`

```bash
# 从根目录下载
tempsave-cli download file.txt

# 从子目录下载
tempsave-cli download file.txt myfolder
```

#### 4. 删除文件 `del <name> [dir]`

```bash
# 删除根目录文件
tempsave-cli del file.txt

# 删除子目录文件
tempsave-cli del file.txt myfolder
```

#### 5. 创建文件夹 `mkdir <dir>`

```bash
# 创建一级目录
tempsave-cli mkdir newfolder

# 创建多级目录
tempsave-cli mkdir "parent/child/grandchild"
```

#### 6. 删除文件夹 `rmdir <dir>`

```bash
# 删除空文件夹
tempsave-cli rmdir emptyfolder

# 注意：文件夹不为空时会删除失败
```

#### 7. 查看存储使用情况 `df`

```bash
tempsave-cli df
# 输出: Used: 512.0 MB / 1.0 GB (50.0%)
```

## 方法二：curl 命令

### 1. 上传文件

```bash
# 上传到根目录
curl -X POST -F "file=@/path/to/file.txt" http://localhost:8088/upload-file

# 上传到子目录
curl -X POST -F "file=@file.txt" -F "dir=myfolder" http://localhost:8088/upload-file

# 上传到多级子目录
curl -X POST -F "file=@file.txt" -F "dir=myfolder/subfolder" http://localhost:8088/upload-file
```

### 2. 获取文件列表

```bash
# 获取根目录文件列表（默认最多 5 个）
curl http://localhost:8088/files

# 获取根目录所有文件
curl http://localhost:8088/files?limit=all

# 获取子目录文件列表
curl "http://localhost:8088/files?limit=all&dir=myfolder"
```

### 3. 下载文件

```bash
# 下载根目录文件
curl -O http://localhost:8088/uploads/file.txt

# 下载子目录文件
curl -O http://localhost:8088/uploads/myfolder/file.txt
```

### 4. 删除文件

```bash
# 删除根目录文件
curl -X POST -d "name=file.txt" http://localhost:8088/del

# 删除子目录文件
curl -X POST -d "name=file.txt&dir=myfolder" http://localhost:8088/del
```

### 5. 创建文件夹

```bash
# 一级目录
curl -X POST -d "dir=newfolder" http://localhost:8088/mkdir

# 多级目录
curl -X POST -d "dir=parent/child" http://localhost:8088/mkdir
```

### 6. 删除文件夹

```bash
# 删除空文件夹
curl -X POST -d "dir=emptyfolder" http://localhost:8088/rmdir
```

### 7. 检查存储空间

```bash
# 检查上传文件所需空间
curl "http://localhost:8088/check-space?size=1048576&name=test.txt"

# 检查上传到子目录所需空间
curl "http://localhost:8088/check-space?size=1048576&name=test.txt&dir=myfolder"
```

## 完整 API 参考

| 端点 | 方法 | 功能 | 参数 |
|------|------|------|------|
| `/files` | GET | 获取文件列表 | `limit` (默认5, `all`=全部), `dir` (子目录) |
| `/upload-file` | POST | 上传文件 | `file` (multipart), `dir` (目标目录，可选) |
| `/uploads/<path>` | GET | 下载文件 | 文件路径（支持子目录） |
| `/del` | POST | 删除文件 | `name`, `dir` (可选) |
| `/mkdir` | POST | 创建文件夹 | `dir` (目录路径) |
| `/rmdir` | POST | 删除文件夹 | `dir` (目录路径，必须为空) |
| `/check-space` | GET | 检查空间 | `size`, `name`, `dir` (可选) |

## 存储限制

- 最大总容量：**1 GB**（递归统计所有子目录）
- 单个文件大小：无限制（受总容量限制）
- 内存缓冲：32 MB（大文件自动流式写入）

## 注意事项

1. **文件覆盖**：上传同名文件会自动覆盖旧文件（同一目录内）
2. **文件夹删除**：只能删除空文件夹，删除前需先清空内部文件
3. **路径安全**：不支持 `..` 路径穿越，不支持绝对路径
4. **文件名编码**：包含特殊字符的文件名需要 URL 编码（CLI 工具自动处理）
5. **网络超时**：大文件上传可能需要调整 curl 超时设置：
   ```bash
   curl --max-time 300 -X POST -F "file=@large_file.zip" http://localhost:8088/upload-file
   ```
6. **路径中的空格**：包含空格的目录名需用引号包裹
