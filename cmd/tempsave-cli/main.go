package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var serverURL string

func main() {
	flag.StringVar(&serverURL, "server", "http://localhost:8088", "Server URL")
	flag.StringVar(&serverURL, "s", "http://localhost:8088", "Server URL (shorthand)")
	flag.Parse()

	if envURL := os.Getenv("TESERVER"); envURL != "" {
		serverURL = envURL
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		return
	}

	switch args[0] {
	case "ls":
		cmdList(args[1:])
	case "upload":
		cmdUpload(args[1:])
	case "download":
		cmdDownload(args[1:])
	case "del":
		cmdDelete(args[1:])
	case "mkdir":
		cmdMkdir(args[1:])
	case "rmdir":
		cmdRmdir(args[1:])
	case "df":
		cmdDiskFree(args[1:])
	default:
		fmt.Printf("Unknown command: %s\n\n", args[0])
		printUsage()
	}
}

func printUsage() {
	fmt.Println(`TempSave CLI - 临时网盘命令行工具

用法:
  tempsave-cli [--server <url>] <command> [args...]

命令:
  ls [dir]              列出文件/文件夹
  upload <file> [dir]   上传文件到指定目录（可选）
  download <name> [dir] 下载文件
  del <name> [dir]      删除文件
  mkdir <dir>           创建文件夹
  rmdir <dir>           删除文件夹
  df                    查看存储使用情况

全局选项:
  --server, -s  指定服务器地址（默认 http://localhost:8088）
                 也可通过 TESERVER 环境变量设置

示例:
  tempsave-cli ls
  tempsave-cli ls myfolder
  tempsave-cli upload file.txt
  tempsave-cli upload file.txt myfolder
  tempsave-cli download file.txt
  tempsave-cli download file.txt myfolder
  tempsave-cli del file.txt
  tempsave-cli del file.txt myfolder
  tempsave-cli mkdir newfolder
  tempsave-cli rmdir emptyfolder
  tempsave-cli df
  tempsave-cli -s http://192.168.1.100:8088 ls`)
}

func cmdList(args []string) {
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	}

	u := fmt.Sprintf("%s/files?limit=all", serverURL)
	if dir != "" {
		u += "&dir=" + dir
	}

	resp, err := http.Get(u)
	if err != nil {
		exitError("Connection failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		exitError("Server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var files []struct {
		Name string `json:"name"`
		Time string `json:"time"`
		Size string `json:"size"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		exitError("Failed to parse response: %v", err)
	}

	if len(files) == 0 {
		fmt.Println("(empty)")
		return
	}

	fmt.Printf("%-4s %-30s %-20s %s\n", "Type", "Name", "Time", "Size")
	fmt.Println(strings.Repeat("-", 80))
	for _, f := range files {
		typeIcon := "FILE"
		if f.Type == "dir" {
			typeIcon = "DIR"
		}
		fmt.Printf("%-4s %-30s %-20s %s\n", typeIcon, f.Name, f.Time, f.Size)
	}
}

func cmdUpload(args []string) {
	if len(args) < 1 {
		exitError("Usage: tempsave-cli upload <filepath> [dir]")
	}

	filePath := args[0]
	dir := ""
	if len(args) > 1 {
		dir = args[1]
	}

	file, err := os.Open(filePath)
	if err != nil {
		exitError("Cannot open file: %v", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		exitError("Cannot stat file: %v", err)
	}

	// Build multipart form
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		writer.WriteField("dir", dir)

		part, err := writer.CreateFormFile("file", filepath.Base(stat.Name()))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		io.Copy(part, file)
	}()

	resp, err := http.Post(serverURL+"/upload-file", writer.FormDataContentType(), pr)
	if err != nil {
		exitError("Upload failed: %v", err)
	}
	defer resp.Body.Close()

	printJSONResponse(resp.Body)
}

func cmdDownload(args []string) {
	if len(args) < 1 {
		exitError("Usage: tempsave-cli download <name> [dir]")
	}

	name := args[0]
	dir := ""
	if len(args) > 1 {
		dir = args[1]
	}

	u := serverURL + "/uploads/"
	if dir != "" {
		u += dir + "/"
	}
	u += name

	resp, err := http.Get(u)
	if err != nil {
		exitError("Download failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		exitError("Server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	outFile, err := os.Create(name)
	if err != nil {
		exitError("Cannot create file: %v", err)
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		exitError("Download failed: %v", err)
	}

	fmt.Printf("Downloaded %s (%d bytes)\n", name, written)
}

func cmdDelete(args []string) {
	if len(args) < 1 {
		exitError("Usage: tempsave-cli del <name> [dir]")
	}

	name := args[0]
	dir := ""
	if len(args) > 1 {
		dir = args[1]
	}

	resp, err := http.PostForm(serverURL+"/del", url.Values{
		"name": {name},
		"dir":  {dir},
	})
	if err != nil {
		exitError("Delete failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Print(string(body))
}

func cmdMkdir(args []string) {
	if len(args) < 1 {
		exitError("Usage: tempsave-cli mkdir <dir>")
	}

	resp, err := http.PostForm(serverURL+"/mkdir", url.Values{
		"dir": {args[0]},
	})
	if err != nil {
		exitError("Mkdir failed: %v", err)
	}
	defer resp.Body.Close()

	printJSONResponse(resp.Body)
}

func cmdRmdir(args []string) {
	if len(args) < 1 {
		exitError("Usage: tempsave-cli rmdir <dir>")
	}

	resp, err := http.PostForm(serverURL+"/rmdir", url.Values{
		"dir": {args[0]},
	})
	if err != nil {
		exitError("Rmdir failed: %v", err)
	}
	defer resp.Body.Close()

	printJSONResponse(resp.Body)
}

func cmdDiskFree(_ []string) {
	resp, err := http.Get(fmt.Sprintf("%s/check-space?size=0&name=", serverURL))
	if err != nil {
		exitError("Failed to get disk info: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Ok            bool   `json:"ok"`
		CurrentSize   int64  `json:"currentSize"`
		AvailableSize int64  `json:"availableSize"`
		MaxSize       int64  `json:"maxSize"`
		Message       string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		exitError("Failed to parse response: %v", err)
	}

	if result.MaxSize > 0 {
		pct := float64(result.CurrentSize) / float64(result.MaxSize) * 100
		fmt.Printf("Used: %s / %s (%.1f%%)\n",
			formatBytes(result.CurrentSize),
			formatBytes(result.MaxSize),
			pct)
	} else {
		fmt.Println(result.Message)
	}
}

func printJSONResponse(r io.Reader) {
	var result map[string]interface{}
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		body, _ := io.ReadAll(r)
		fmt.Println(string(body))
		return
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
}

func formatBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	size := float64(n)
	idx := 0
	for size >= 1024 && idx < len(units)-1 {
		size /= 1024
		idx++
	}
	return fmt.Sprintf("%.1f %s", size, units[idx])
}

func exitError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
