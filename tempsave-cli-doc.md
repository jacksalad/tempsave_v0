# TempSave CLI 使用文档

本文档介绍如何通过命令行（curl）操作 TempSave 临时网盘服务。

## 前提条件

- 服务已启动（默认端口 8088）
- 已安装 curl 工具

## 快速开始

### 1. 上传文件

```bash
curl -X POST -F "file=@/path/to/file.txt" http://localhost:8088/upload-file
```

**参数说明：**
- `-X POST` - 使用 POST 方法
- `-F "file=@/path/to/file.txt"` - 指定要上传的文件（`@` 符号表示读取文件）
- `http://localhost:8088/upload-file` - 上传接口地址

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

**失败响应（空间不足）：**
```json
{
  "ok": false,
  "message": "空间不足！需要 500.00 MB，剩余 200.00 MB",
  "requiredSize": 524288000,
  "availableSize": 209715200
}
```

### 2. 获取文件列表

```bash
# 获取文件列表（默认最多 5 个）
curl http://localhost:8088/files

# 获取所有文件列表
curl http://localhost:8088/files?limit=all
```

**响应示例：**
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

### 3. 下载文件

```bash
# 下载并保存为原文件名
curl -O http://localhost:8088/uploads/file.txt

# 下载并指定保存文件名
curl -o my_file.txt http://localhost:8088/uploads/file.txt
```

**参数说明：**
- `-O` - 使用服务器上的文件名保存
- `-o <filename>` - 指定保存的文件名

### 4. 删除文件

```bash
curl "http://localhost:8088/del?name=file.txt"
```

**注意：** 文件名需要进行 URL 编码，如果文件名包含特殊字符或中文：

```bash
# 使用 --data-urlencode 自动编码
curl -G "http://localhost:8088/del" --data-urlencode "name=文件 名.txt"
```

**响应：**
```
file.txt deleted.
```

### 5. 检查存储空间

```bash
curl "http://localhost:8088/check-space?size=1048576&name=test.txt"
```

**参数说明：**
- `size` - 要上传的文件大小（字节）
- `name` - 文件名（用于检查是否覆盖同名文件）

**空间充足响应：**
```json
{
  "ok": true,
  "message": "空间充足",
  "currentSize": 524288000,
  "availableSize": 524288000,
  "maxSize": 1073741824
}
```

**空间不足响应：**
```json
{
  "ok": false,
  "message": "空间不足！需要 100.00 MB，剩余 50.00 MB",
  "currentSize": 1023410176,
  "availableSize": 52428800,
  "maxSize": 1073741824
}
```

## 完整 API 参考

| 端点 | 方法 | 功能 | 参数 |
|------|------|------|------|
| `/upload-file` | POST | 上传文件 | `file` (multipart/form-data) |
| `/files` | GET | 获取文件列表 | `limit` (query参数，默认5，设为all获取全部) |
| `/uploads/<filename>` | GET | 下载文件 | 文件名在 URL 路径中 |
| `/del` | GET | 删除文件 | `name` (query参数) |
| `/check-space` | GET | 检查空间 | `size`, `name` (query参数) |

## 高级用法

### 批量上传文件

```bash
# 循环上传当前目录下所有 .txt 文件
for file in *.txt; do
  echo "Uploading $file..."
  curl -X POST -F "file=@$file" http://localhost:8088/upload-file
  echo
done
```

### 显示上传进度

```bash
curl -X POST -F "file=@large_file.zip" --progress-bar http://localhost:8088/upload-file
```

### 上传并显示详细信息

```bash
curl -X POST -F "file=@file.txt" -w "\nHTTP Status: %{http_code}\nTime: %{time_total}s\n" http://localhost:8088/upload-file
```

### 使用变量配置服务器地址

```bash
SERVER="http://192.168.1.100:8088"

# 上传
curl -X POST -F "file=@file.txt" $SERVER/upload-file

# 获取列表
curl $SERVER/files
```

## 错误处理

| HTTP 状态码 | 说明 |
|------------|------|
| 200 | 成功 |
| 400 | 请求参数错误（缺少文件或无效文件） |
| 405 | 方法不允许（如使用 GET 访问 POST 接口） |
| 500 | 服务器内部错误 |
| 507 | 存储空间不足 |

## 存储限制

- 最大总容量：**1 GB**
- 单个文件大小：无限制（受总容量限制）
- 内存缓冲：32 MB（大文件自动流式写入）

## 注意事项

1. **文件覆盖**：上传同名文件会自动覆盖旧文件
2. **文件名编码**：包含特殊字符的文件名需要 URL 编码
3. **网络超时**：大文件上传可能需要调整 curl 超时设置：
   ```bash
   curl --max-time 300 -X POST -F "file=@large_file.zip" http://localhost:8088/upload-file
   ```
