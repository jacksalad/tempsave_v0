# TempSave

一个轻量的临时文件共享服务，支持 Web 界面和 CLI 操作，支持分片上传大文件，支持文件夹管理，并有 1GB 的存储限制。

## 快速开始

```bash
# 运行服务（端口 8088）
go run main.go

# 构建可执行文件
go build -o tempsave.exe

# 构建 CLI 工具
go build -o tempsave-cli.exe ./cmd/tempsave-cli/
```

启动后访问 http://localhost:8088 即可使用 Web 界面。

## 功能特性

- 📂 **文件夹管理** — 在 Web 界面中创建、进入、删除文件夹，上传到指定目录
- 📤 **分片上传** — 支持大文件分片上传（前端 1MB 切片）
- 📥 **文件下载** — 支持子目录文件访问
- 🗑️ **文件/文件夹删除** — 删除文件或清空文件夹
- 💾 **存储上限 1GB** — 递归统计所有子目录
- 🖥️ **Go CLI 工具** — 独立的命令行客户端，支持所有操作

## Web 界面

- 拖拽上传或点击上传按钮
- 文件夹导航：点击文件夹进入，面包屑返回
- "+ 新建文件夹" 按钮创建目录
- 上传到当前所在目录

## CLI 使用

### Go CLI 工具

```bash
# 列出文件
tempsave-cli ls [dir]

# 上传文件
tempsave-cli upload file.txt [dir]

# 下载文件
tempsave-cli download name [dir]

# 删除文件
tempsave-cli del name [dir]

# 创建文件夹
tempsave-cli mkdir dir

# 删除文件夹
tempsave-cli rmdir dir

# 查看存储空间
tempsave-cli df
```

### curl

```bash
# 上传文件到根目录
curl -X POST -F "file=@file.txt" http://localhost:8088/upload-file

# 上传到子目录
curl -X POST -F "file=@file.txt" -F "dir=myfolder" http://localhost:8088/upload-file

# 获取文件列表（全部）
curl "http://localhost:8088/files?limit=all"

# 获取子目录文件列表
curl "http://localhost:8088/files?limit=all&dir=myfolder"

# 下载文件
curl -O http://localhost:8088/uploads/file.txt
curl -O http://localhost:8088/uploads/myfolder/file.txt

# 删除文件
curl -X POST -d "name=file.txt" http://localhost:8088/del
curl -X POST -d "name=file.txt&dir=myfolder" http://localhost:8088/del

# 创建文件夹
curl -X POST -d "dir=newfolder" http://localhost:8088/mkdir

# 删除文件夹
curl -X POST -d "dir=emptyfolder" http://localhost:8088/rmdir
```

## API 接口

| 路由 | 方法 | 说明 |
|------|------|------|
| `/files?limit=&dir=` | GET | 文件列表（`limit=all` 获取全部，`dir` 指定目录） |
| `/upload-file` | POST | 上传文件（`file` + 可选 `dir`） |
| `/upload` | POST | 分片上传（前端专用，支持 `dir`） |
| `/uploads/{path}` | GET | 下载文件（支持子目录路径） |
| `/del` | POST | 删除文件（form: `name` + 可选 `dir`） |
| `/mkdir` | POST | 创建文件夹（form: `dir`） |
| `/rmdir` | POST | 删除空文件夹（form: `dir`） |
| `/check-space` | GET | 检查空间（`size`, `name`, 可选 `dir`） |

## 目录结构

```
uploads/    # 上传文件存储（自动创建子目录）
app.log     # 运行日志
```
