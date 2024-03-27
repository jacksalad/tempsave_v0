package main

import (
	"log"
	"net/http"
	"os"
	"sync"
	"encoding/json"
	"io/ioutil"
	"fmt"
	"io"
	"time"
	"strconv"
	"path/filepath"
)

const (
	port       string = "8088"              // 设置端口
	timeLayout string = "2006-01-02 3:04PM" // 时间格式化
)

var (
	uploadDir string     = "./src/assets/uploads" // 文件上传目录
	lock      sync.Mutex                          // 用于文件合并时的线程安全锁
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

	// 设置静态资源目录以及主页路由
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("src/assets"))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		http.ServeFile(w, r, "src/index.html")
		log.Println("【Enter】 Web is entered")
	})

	// 获取文件列表
	http.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		files, err := ioutil.ReadDir(uploadDir)
		if err != nil {
			log.Println("Error reading files directory")
			return
		}
		var fileNames []FileIfo
		for _, file := range files {
			fileNames = append(fileNames, FileIfo{file.Name(), file.ModTime().Format(timeLayout), SizeFormat(file.Size())})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(fileNames)
	})

	// 合并文件
	mergeFile := func(fileName string, chunks int) {
		lock.Lock()
		defer lock.Unlock()
		// 创建最终的文件
		mergedFile, err := os.Create(filepath.Join(uploadDir, fileName))
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
		time.Sleep(time.Second)
		os.Remove(uploadDir + "/_0")
	}

	// 上传文件函数
	handleUpload := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		// 解析表单数据
		err := r.ParseMultipartForm(1000 << 20) // 限制最大内存1GB
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
		// 获取文件名和当前切片信息
		fileName := r.FormValue("fileName")
		chunk, _ := strconv.Atoi(r.FormValue("chunk"))
		chunks, _ := strconv.Atoi(r.FormValue("chunks"))
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
			go mergeFile(fileName, chunks)
		}
	}
	// 处理上传文件
	http.HandleFunc("/upload", handleUpload)

	// 删除文件请求
	http.HandleFunc("/del", func(w http.ResponseWriter, r *http.Request) {
		filename := r.URL.Query().Get("name")
		os.Remove(filepath.Join(uploadDir, filename))
		fmt.Fprintln(w, filename+" deleted.")
		log.Printf("【Delete】 delete %s successfully\n", filename)
	})

	log.Printf("【Start】 localhost:%s", port)
	if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
