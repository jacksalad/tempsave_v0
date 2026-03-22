package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	portstring = "8088"              // 设置端口
	timeLayout string = "2006-01-02 3:04PM" // 时间格式化
	maxTotalSize int64 = 1 << 30// 最大总容量 1GB
)

var (
	uploadDir string     = "./uploads" // 文件上传目录
	lock      sync.Mutex               // 用于文件合并时的线程安全锁
	cachedTotalSize int64              // 缓存的总大小（原子操作）
	sizeCacheReady bool                // 缓存是否已初始化
)

// 文件信息结构体
type FileIfo struct {
	Name string `json:"name"`
	Time string `json:"time"`
	Size string `json:"size"`
}

// 文件大小的字符串表达式
func SizeFormat(fileSize int64) string {
	size := float64(fileSize)
	units := []string{"Bytes", "KB", "MB", "GB"}
	unitIndex := 0
	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}
	return fmt.Sprintf("%.2f %s", size, units[unitIndex])
}

// 计算并更新缓存的总大小（内部函数）
func calculateTotalSize() (int64, error) {
	var totalSize int64
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			// 只计算最终文件（不含切片临时文件）
			name := entry.Name()
			if !isChunkFile(name) {
				totalSize += info.Size()
			}
		}
	}
	return totalSize, nil
}

// 判断是否为切片临时文件
func isChunkFile(name string) bool {
	// 切片文件格式: filename_chunkIndex
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '_' {
			// 检查后面是否全是数字
			for j := i + 1; j < len(name); j++ {
				if name[j] < '0' || name[j] > '9' {
					return false
				}
			}
			return true
		}
	}
	return false
}

// 初始化缓存（启动时调用）
func initSizeCache() {
	size, err := calculateTotalSize()
	if err != nil {
		log.Printf("Failed to initialize size cache: %v\n", err)
		return
	}
	atomic.StoreInt64(&cachedTotalSize, size)
	sizeCacheReady = true
}

// 获取缓存的总大小（优先返回缓存，缓存未初始化时计算）
func getTotalSize() (int64, error) {
	if sizeCacheReady {
		return atomic.LoadInt64(&cachedTotalSize), nil
	}
	// 缓存未初始化时，直接计算
	return calculateTotalSize()
}

// 更新缓存（文件大小变化时调用）
func updateSizeCache(delta int64) {
	if sizeCacheReady {
		atomic.AddInt64(&cachedTotalSize, delta)
	}
}

// 重新计算并更新缓存（删除文件后调用，因为可能删除的是未知大小的文件）
func refreshSizeCache() {
	size, err := calculateTotalSize()
	if err == nil {
		atomic.StoreInt64(&cachedTotalSize, size)
	}
}

func main() {
	// 创建或打开日志文件
	file, err := os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()
	log.SetOutput(file)

	// 处理上传文件夹
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		// 文件夹不存在，创建文件夹
		err := os.Mkdir(uploadDir, 0755)
		if err != nil {
			log.Println("Error creating directory:", err)
			return
		}
		log.Println("Directory created successfully!")
	}

	// 初始化大小缓存
	initSizeCache()

	// 设置静态资源目录
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("src/assets"))))
	// 设置上传文件目录
	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	// 主页路由
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		http.ServeFile(w, r, "src/index.html")
		log.Println("【Enter】 Web is entered")
	})

	// 获取文件列表
	http.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(uploadDir)
		if err != nil {
			log.Println("Error reading files directory")
			return
		}
		// 过滤掉切片临时文件，只保留最终文件
		type fileInfo struct {
			name    string
			modTime time.Time
			size    int64
		}
		var files []fileInfo
		for _, entry := range entries {
			if !entry.IsDir() && !isChunkFile(entry.Name()) {
				info, err := entry.Info()
				if err != nil {
					continue
				}
				files = append(files, fileInfo{
					name:    entry.Name(),
					modTime: info.ModTime(),
					size:    info.Size(),
				})
			}
		}
		// 按修改时间排序，最新的在前
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.After(files[j].modTime)
		})
		var fileNames []FileIfo
		for _, file := range files {
			fileNames = append(fileNames, FileIfo{file.name, file.modTime.Format(timeLayout), SizeFormat(file.size)})
		}
		// 处理 limit 参数
		limitParam := r.URL.Query().Get("limit")
		if limitParam != "all" {
			// 默认返回最多 5 个文件
			limit := 5
			if limitParam != "" {
				if customLimit, err := strconv.Atoi(limitParam); err == nil && customLimit > 0 {
					limit = customLimit
				}
			}
			if len(fileNames) > limit {
				fileNames = fileNames[:limit]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(fileNames)
	})

	// 合并文件
	mergeFile := func(fileName string, chunks int, fileSize int64) {
		lock.Lock()
		defer lock.Unlock()
		// 安全处理文件名，防止路径遍历攻击
		fileName = filepath.Base(fileName)
		// 检查空间是否足够
		currentSize, err := getTotalSize()
		if err != nil {
			log.Printf("Failed to get total size: %v\n", err)
			// 清理切片文件
			for i := 0; i < chunks; i++ {
				partFileName := fmt.Sprintf("%s/%s_%d", uploadDir, fileName, i)
				os.Remove(partFileName)
			}
			return
		}
		// 如果同名文件已存在，先删除（覆盖旧文件，需要减去旧文件大小）
		existingFile := filepath.Join(uploadDir, fileName)
		if oldInfo, err := os.Stat(existingFile); err == nil {
			currentSize -= oldInfo.Size()
		}
		// 检查上传后是否会超过限制
		if currentSize+fileSize > maxTotalSize {
			log.Printf("【Upload Failed】 Insufficient space: current %d + new %d > limit %d\n", currentSize, fileSize, maxTotalSize)
			// 清理切片文件
			for i := 0; i < chunks; i++ {
				partFileName := fmt.Sprintf("%s/%s_%d", uploadDir, fileName, i)
				os.Remove(partFileName)
			}
			return
		}
		// 删除已存在的同名文件
		os.Remove(existingFile)
		// 创建最终的文件
		mergedFile, err := os.Create(existingFile)
		if err != nil {
			log.Printf("Failed to create merged file: %v\n", err)
			return
		}
		defer mergedFile.Close()
		// 按顺序读取每个切片，并写入最终文件
		for i := 0; i < chunks; i++ {
			partFileName := fmt.Sprintf("%s/%s_%d", uploadDir, fileName, i)
			partFile, err := os.Open(partFileName)
			if err != nil {
				log.Printf("Failed to open part file %s: %v\n", partFileName, err)
				return
			}
			_, err = io.Copy(mergedFile, partFile)
			partFile.Close()
			if err != nil {
				log.Printf("Failed to append part file %s to merged file: %v\n", partFileName, err)
				return
			}
			// 删除切片文件
			os.Remove(partFileName)
		}
		log.Printf("【Upload】 File %s Uploaded successfully\n", fileName)
		// 更新缓存：增加新文件大小
		updateSizeCache(fileSize)
	}

	// 上传文件函数
	handleUpload := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		// 解析表单数据（32MB 缓冲区，分片只有1MB不需要更大）
		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
			return
		}
		// 获取文件切片
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Invalid file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		// 获取文件名和当前切片信息（安全处理文件名，防止路径遍历攻击）
		fileName := filepath.Base(r.FormValue("fileName"))
		chunk, _ := strconv.Atoi(r.FormValue("chunk"))
		chunks, _ := strconv.Atoi(r.FormValue("chunks"))
		fileSize, _ := strconv.ParseInt(r.FormValue("fileSize"), 10, 64)
		// 第一个切片时检查空间
		if chunk == 0 {
			currentSize, _ := getTotalSize()
			// 如果同名文件已存在，减去其大小
			existingFile := filepath.Join(uploadDir, fileName)
			if oldInfo, err := os.Stat(existingFile); err == nil {
				currentSize -= oldInfo.Size()
			}
			if currentSize+fileSize > maxTotalSize {
				http.Error(w, "Insufficient storage space", http.StatusInsufficientStorage)
				log.Printf("【Upload Rejected】 Insufficient space for %s: need %d, available %d\n", fileName, fileSize, maxTotalSize-currentSize)
				return
			}
		}
		// 确保上传目录存在
		os.MkdirAll(uploadDir, os.ModePerm)
		// 创建/打开当前切片的文件
		partFileName := fmt.Sprintf("%s/%s_%d", uploadDir, fileName, chunk)
		partFile, err := os.Create(partFileName)
		if err != nil {
			http.Error(w, "Failed to create part file", http.StatusInternalServerError)
			return
		}
		defer partFile.Close()
		// 将切片内容写入文件
		_, err = io.Copy(partFile, file)
		if err != nil {
			http.Error(w, "Failed to write part file", http.StatusInternalServerError)
			return
		}
		// 如果是最后一个切片，则开始合并文件
		if chunk == chunks-1 {
			go mergeFile(fileName, chunks, fileSize)
		}
	}
	// 处理上传文件
	http.HandleFunc("/upload", handleUpload)

	// 检查空间是否足够（上传前预检查）
	http.HandleFunc("/check-space", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		
		fileSizeStr := r.URL.Query().Get("size")
		fileName := filepath.Base(r.URL.Query().Get("name")) // 安全处理文件名，防止路径遍历攻击
		if fileSizeStr == ""{
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": false,
				"message": "缺少文件大小参数",
			})
			return
		}
		fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)
		
		currentSize, err := getTotalSize()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": false,
				"message": "无法获取当前存储使用情况",
			})
			return
		}
		
		// 如果同名文件已存在，减去其大小（覆盖场景）
		existingFile := filepath.Join(uploadDir, fileName)
		if oldInfo, err := os.Stat(existingFile); err == nil {
			currentSize -= oldInfo.Size()
		}
		
		available := maxTotalSize - currentSize
		if currentSize + fileSize > maxTotalSize {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": false,
				"message": fmt.Sprintf("空间不足！需要 %s，剩余 %s", SizeFormat(fileSize), SizeFormat(available)),
				"currentSize": currentSize,
				"availableSize": available,
				"maxSize": maxTotalSize,
			})
			return
		}
		
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"message": "空间充足",
			"currentSize": currentSize,
			"availableSize": available,
			"maxSize": maxTotalSize,
		})
	})

	// 删除文件请求
	http.HandleFunc("/del", func(w http.ResponseWriter, r *http.Request) {
		filename := filepath.Base(r.URL.Query().Get("name")) // 安全处理文件名，防止路径遍历攻击
		filePath := filepath.Join(uploadDir, filename)
		// 获取文件大小用于更新缓存
		var oldSize int64
		if info, err := os.Stat(filePath); err == nil {
			oldSize = info.Size()
		}
		os.Remove(filePath)
		// 更新缓存：减少文件大小
		updateSizeCache(-oldSize)
		fmt.Fprintln(w, filename+" deleted.")
		log.Printf("【Delete】 delete %s successfully\n", filename)
	})

	// CLI 单文件上传接口（支持大文件分片写入）
	http.HandleFunc("/upload-file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "仅支持 POST 方法",
			})
			return
		}

		// 限制内存缓冲区大小（32MB），大文件直接写入磁盘
		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "无法解析表单数据: " + err.Error(),
			})
			return
		}

		// 获取上传的文件
		file, handler, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "无效的文件: " + err.Error(),
			})
			return
		}
		defer file.Close()

		fileName := filepath.Base(handler.Filename) // 安全处理文件名，防止路径遍历攻击
		fileSize := handler.Size

		// 检查存储空间
		currentSize, err := getTotalSize()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "无法获取存储使用情况",
			})
			return
		}

		// 如果同名文件已存在，减去其大小（覆盖场景）
		existingFile := filepath.Join(uploadDir, fileName)
		if oldInfo, err := os.Stat(existingFile); err == nil {
			currentSize -= oldInfo.Size()
		}

		available := maxTotalSize - currentSize
		if currentSize+fileSize > maxTotalSize {
			w.WriteHeader(http.StatusInsufficientStorage)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":           false,
				"message":      fmt.Sprintf("空间不足！需要 %s，剩余 %s", SizeFormat(fileSize), SizeFormat(available)),
				"requiredSize": fileSize,
				"availableSize": available,
			})
			log.Printf("【Upload-CLI Rejected】 Insufficient space for %s: need %d, available %d\n", fileName, fileSize, available)
			return
		}

		// 删除已存在的同名文件
		os.Remove(existingFile)

		// 创建目标文件
		dst, err := os.Create(existingFile)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "无法创建文件: " + err.Error(),
			})
			return
		}
		defer dst.Close()

		// 使用 1MB 缓冲区分片写入（性能优化）
		buf := make([]byte, 1<<20) // 1MB buffer
		written, err := io.CopyBuffer(dst, file, buf)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "文件写入失败: " + err.Error(),
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"message": "上传成功",
			"fileName": fileName,
			"size":     SizeFormat(written),
			"bytes":    written,
		})
		log.Printf("【Upload-CLI】 File %s (%s) uploaded successfully\n", fileName, SizeFormat(written))
	})

	log.Printf("【Start】 Server running at http://localhost:%s", portstring)
	fmt.Printf("Server running at http://localhost:%s\n", portstring)
	if err := http.ListenAndServe("0.0.0.0:"+portstring, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
