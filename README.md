# TempSave

一个轻量的临时文件共享服务，支持 Web 界面和 CLI 操作。

![](https://github.com/jacksalad/tempsave_v0/blob/main/src/assets/img/screenshot.png?raw=true)

## 快速开始

```bash
# 运行服务（端口 8088）
go run main.go

# 构建可执行文件
go build -o tempsave.exe
```

启动后访问 http://localhost:8088 即可使用 Web 界面。

## 功能特性

- 📤 分片上传（支持大文件）
- 📥 文件下载
- 🗑️ 文件删除
- 💾 存储上限 1GB

## CLI 使用

```bash
# 上传文件
curl -X POST -F "file=@file.txt" http://localhost:8088/upload-file

# 获取文件列表
curl http://localhost:8088/files?limit=all

# 下载文件
curl -O http://localhost:8088/uploads/file.txt

# 删除文件
curl "http://localhost:8088/del?name=file.txt"
```

## API 接口

| 路由 | 方法 | 说明 |
|------|------|------|
| `/files` | GET | 文件列表（`?limit=all` 获取全部） |
| `/upload-file` | POST | 上传文件 |
| `/uploads/{name}` | GET | 下载文件 |
| `/del?name=` | GET | 删除文件 |

## 目录结构

```
uploads/    # 上传文件存储
app.log     # 运行日志
```
