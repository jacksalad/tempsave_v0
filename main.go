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
	portstring     string = "8088"                // 设置端口
	timeLayout     string = "2006-01-02 3:04PM"   // 时间格式化
	maxTotalSize   int64  = 1 << 30               // 最大总容量 1GB
	filesCacheTTL         = 2 * time.Second       // 文件列表缓存 TTL
)

var (
	uploadDir      string = "./uploads" // 文件上传目录
	cachedTotalSize int64               // 缓存的总大小（原子操作）
	sizeCacheReady atomic.Bool          // 缓存是否已初始化（原子操作，P0 修复）
	sizeCacheOnce  sync.Once            // 确保只初始化一次
	fileLocks      sync.Map             // 按文件名加锁（P2 修复，替代全局 mutex）
	filesCache     []FileIfo            // 文件列表缓存（P3 修复）
	filesCacheTime time.Time            // 文件列表缓存时间
	filesCacheMu   sync.RWMutex         // 文件列表缓存锁
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

// 初始化缓存（启动时调用，P0 修复：使用 sync.Once + atomic.Bool）
func initSizeCache() {
	sizeCacheOnce.Do(func() {
		size, err := calculateTotalSize()
		if err != nil {
			log.Printf("Failed to initialize size cache: %v\n", err)
			return
		}
		atomic.StoreInt64(&cachedTotalSize, size)
		sizeCacheReady.Store(true)
	})
}

// 获取缓存的总大小（优先返回缓存，缓存未初始化时计算）
func getTotalSize() (int64, error) {
	if sizeCacheReady.Load() {
		return atomic.LoadInt64(&cachedTotalSize), nil
	}
	// 缓存未初始化时，直接计算
	return calculateTotalSize()
}

// 更新缓存（文件大小变化时调用）
func updateSizeCache(delta int64) {
	if sizeCacheReady.Load() {
		atomic.AddInt64(&cachedTotalSize, delta)
	}
}

// 重新计算并更新缓存
func refreshSizeCache() {
	size, err := calculateTotalSize()
	if err == nil {
		atomic.StoreInt64(&cachedTotalSize, size)
	}
}

// getFileLock 获取按文件名的锁（P2 修复：替代全局 mutex）
func getFileLock(name string) *sync.Mutex {
	v, _ := fileLocks.LoadOrStore(name, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// checkSpace 检查存储空间是否足够（P3 修复：抽取公共函数）
func checkSpace(fileName string, fileSize int64) (available int64, ok bool, errMsg string) {
	currentSize, err := getTotalSize()
	if err != nil {
		return 0, false, "无法获取存储使用情况"
	}
	existingFile := filepath.Join(uploadDir, fileName)
	if oldInfo, err := os.Stat(existingFile); err == nil {
		currentSize -= oldInfo.Size()
	}
	available = maxTotalSize - currentSize
	if currentSize+fileSize > maxTotalSize {
		return available, false, fmt.Sprintf("空间不足！需要 %s，剩余 %s", SizeFormat(fileSize), SizeFormat(available))
	}
	return available, true, ""
}

// invalidateFilesCache 失效文件列表缓存
func invalidateFilesCache() {
	filesCacheMu.Lock()
	filesCache = nil
	filesCacheMu.Unlock()
}

// readFilesFromDisk 从磁盘读取文件列表
func readFilesFromDisk() ([]FileIfo, error) {
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return nil, err
	}
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
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	result := make([]FileIfo, len(files))
	for i, file := range files {
		result[i] = FileIfo{file.name, file.modTime.Format(timeLayout), SizeFormat(file.size)}
	}
	return result, nil
}

// getCachedFiles 获取文件列表（带 2 秒 TTL 缓存，P3 修复）
func getCachedFiles() ([]FileIfo, error) {
	filesCacheMu.RLock()
	if time.Since(filesCacheTime) < filesCacheTTL && filesCache != nil {
		result := filesCache
		filesCacheMu.RUnlock()
		return result, nil
	}
	filesCacheMu.RUnlock()

	files, err := readFilesFromDisk()
	if err != nil {
		return nil, err
	}

	filesCacheMu.Lock()
	filesCache = files
	filesCacheTime = time.Now()
	filesCacheMu.Unlock()
	return files, nil
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

	// 获取文件列表（P3 修复：使用缓存）
	http.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		fileNames, err := getCachedFiles()
		if err != nil {
			log.Println("Error reading files directory")
			return
		}
		// 处理 limit 参数
		limitParam := r.URL.Query().Get("limit")
		if limitParam != "all" {
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

	// 合并文件（P1 修复：失败回滚清理 + 1MB buffer；P2 修复：按文件名加锁）
	mergeFile := func(fileName string, chunks int, fileSize int64) {
		fileName = filepath.Base(fileName)

		// 按文件名加锁（P2 修复）
		fl := getFileLock(fileName)
		fl.Lock()
		defer fl.Unlock()
		defer fileLocks.Delete(fileName) // 用完后删除，避免内存泄漏

		// 检查空间是否足够（使用公共函数，P3 修复）
		_, ok, _ := checkSpace(fileName, fileSize)
		if !ok {
			log.Printf("【Upload Failed】 Insufficient space for %s\n", fileName)
			// 清理切片文件
			for i := 0; i < chunks; i++ {
				os.Remove(fmt.Sprintf("%s/%s_%d", uploadDir, fileName, i))
			}
			return
		}

		existingFile := filepath.Join(uploadDir, fileName)
		// 删除已存在的同名文件
		os.Remove(existingFile)

		// 创建最终的文件
		mergedFile, err := os.Create(existingFile)
		if err != nil {
			log.Printf("Failed to create merged file: %v\n", err)
			// 清理切片文件
			for i := 0; i < chunks; i++ {
				os.Remove(fmt.Sprintf("%s/%s_%d", uploadDir, fileName, i))
			}
			return
		}

		// P1 修复：失败时回滚清理残留文件
		success := false
		defer func() {
			mergedFile.Close()
			if !success {
				// 合并失败，删除不完整文件
				os.Remove(existingFile)
				// 清理剩余切片文件
				for i := 0; i < chunks; i++ {
					os.Remove(fmt.Sprintf("%s/%s_%d", uploadDir, fileName, i))
				}
				// 刷新缓存（可能删了旧文件）
				refreshSizeCache()
			}
		}()

		// P1 修复：使用 1MB 缓冲区
		buf := make([]byte, 1<<20)
		for i := 0; i < chunks; i++ {
			partFileName := fmt.Sprintf("%s/%s_%d", uploadDir, fileName, i)
			partFile, err := os.Open(partFileName)
			if err != nil {
				log.Printf("Failed to open part file %s: %v\n", partFileName, err)
				return
			}
			_, err = io.CopyBuffer(mergedFile, partFile, buf)
			partFile.Close()
			os.Remove(partFileName) // 成功合并的切片立即删除
			if err != nil {
				log.Printf("Failed to append part file %s to merged file: %v\n", partFileName, err)
				return
			}
		}
		success = true
		updateSizeCache(fileSize)
		invalidateFilesCache() // 失效文件列表缓存
		log.Printf("【Upload】 File %s Uploaded successfully\n", fileName)
	}

	// 上传文件函数
	handleUpload := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Invalid file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileName := filepath.Base(r.FormValue("fileName"))
		chunk, _ := strconv.Atoi(r.FormValue("chunk"))
		chunks, _ := strconv.Atoi(r.FormValue("chunks"))
		fileSize, _ := strconv.ParseInt(r.FormValue("fileSize"), 10, 64)
		// 第一个切片时检查空间（使用公共函数，P3 修复）
		if chunk == 0 {
			_, ok, errMsg := checkSpace(fileName, fileSize)
			if !ok {
				http.Error(w, errMsg, http.StatusInsufficientStorage)
				log.Printf("【Upload Rejected】 %s: %s\n", fileName, errMsg)
				return
			}
		}
		os.MkdirAll(uploadDir, os.ModePerm)
		// 创建/打开当前切片的文件
		partFileName := fmt.Sprintf("%s/%s_%d", uploadDir, fileName, chunk)
		partFile, err := os.Create(partFileName)
		if err != nil {
			http.Error(w, "Failed to create part file", http.StatusInternalServerError)
			return
		}
		// P0 修复：显式关闭文件后再启动 goroutine，避免竞态
		_, err = io.Copy(partFile, file)
		partFile.Close()
		if err != nil {
			http.Error(w, "Failed to write part file", http.StatusInternalServerError)
			return
		}
		if chunk == chunks-1 {
			go mergeFile(fileName, chunks, fileSize)
		}
	}
	http.HandleFunc("/upload", handleUpload)

	// 检查空间是否足够（使用公共函数，P3 修复）
	http.HandleFunc("/check-space", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		fileSizeStr := r.URL.Query().Get("size")
		fileName := filepath.Base(r.URL.Query().Get("name"))
		if fileSizeStr == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "缺少文件大小参数",
			})
			return
		}
		fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)

		currentSize, err := getTotalSize()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
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
		if currentSize+fileSize > maxTotalSize {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":           false,
				"message":      fmt.Sprintf("空间不足！需要 %s，剩余 %s", SizeFormat(fileSize), SizeFormat(available)),
				"currentSize":  currentSize,
				"availableSize": available,
				"maxSize":      maxTotalSize,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":            true,
			"message":       "空间充足",
			"currentSize":   currentSize,
			"availableSize": available,
			"maxSize":       maxTotalSize,
		})
	})

	// 删除文件请求（P3 修复：改用 POST 方法）
	http.HandleFunc("/del", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		filename := filepath.Base(r.FormValue("name"))
		filePath := filepath.Join(uploadDir, filename)
		var oldSize int64
		if info, err := os.Stat(filePath); err == nil {
			oldSize = info.Size()
		}
		os.Remove(filePath)
		updateSizeCache(-oldSize)
		invalidateFilesCache() // 失效文件列表缓存
		fmt.Fprintln(w, filename+" deleted.")
		log.Printf("【Delete】 delete %s successfully\n", filename)
	})

	// CLI 单文件上传接口（使用公共空间检查函数，P3 修复）
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

		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "无法解析表单数据: " + err.Error(),
			})
			return
		}

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

		fileName := filepath.Base(handler.Filename)
		fileSize := handler.Size

		// 使用公共空间检查函数
		available, ok, errMsg := checkSpace(fileName, fileSize)
		if !ok {
			w.WriteHeader(http.StatusInsufficientStorage)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":            false,
				"message":       errMsg,
				"requiredSize":  fileSize,
				"availableSize": available,
			})
			log.Printf("【Upload-CLI Rejected】 Insufficient space for %s: need %d, available %d\n", fileName, fileSize, available)
			return
		}

		existingFile := filepath.Join(uploadDir, fileName)
		os.Remove(existingFile)

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

		buf := make([]byte, 1<<20)
		written, err := io.CopyBuffer(dst, file, buf)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"message": "文件写入失败: " + err.Error(),
			})
			return
		}

		updateSizeCache(written)
		invalidateFilesCache() // 失效文件列表缓存
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":       true,
			"message":  "上传成功",
			"fileName": fileName,
			"size":     SizeFormat(written),
			"bytes":    written,
		})
		log.Printf("【Upload-CLI】 File %s (%s) uploaded successfully\n", fileName, SizeFormat(written))
	})

	// P2 修复：配置 HTTP 超时
	log.Printf("【Start】 Server running at http://localhost:%s", portstring)
	fmt.Printf("Server running at http://localhost:%s\n", portstring)
	srv := &http.Server{
		Addr:           "0.0.0.0:" + portstring,
		ReadTimeout:    0,              // 不限制读超时（大文件上传需要长时间）
		WriteTimeout:   120 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB header 限制
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
