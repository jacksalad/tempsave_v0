package main

// 访问日志（access log）
//
// 2026-10-12 新增。目的：让每一次 HTTP 请求都可追溯，用于判断是否存在
// 扫描、枚举、暴力破解密码、异常下载等非法访问。
//
// 记录内容（每条一行，写进 access.log，按大小自动轮转）：
//   [ACCESS] <客户端IP> <方法> "<路径?查询>" <协议> -> <状态码> <字节数> <耗时>
//            auth=<是否带有效授权票> len=<请求体大小> ref="<Referer>" ua="<User-Agent>"
//
// 接入方式：只在 http.Server 上包一层 Handler（main.go 里一行），
// 因此所有路由——页面、/assets/、/uploads/、以及全部功能接口——
// 都会被记录，而各个 handler 本身一行代码都没改，接口契约不变。
//
// 关于客户端 IP：直接对外暴露时 RemoteAddr 就是真实来源；
// 若以后前面套了反代（nginx/Caddy），会优先取 X-Forwarded-For / X-Real-IP 的首跳。
// 注意这两个头客户端可伪造，仅在确认真有反代时才可信。

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	accessLogFile    = "access.log"    // 访问日志文件名（相对 cwd，与 app.log 同目录）
	accessLogMaxSize = 8 * 1024 * 1024 // 单个日志文件上限 8MB，超过就轮转
	accessLogBackups = 2               // 保留 access.log.1 / access.log.2
)

// accessLogger 独立于标准 log（那个写 app.log），避免两种日志互相干扰
var accessLogger *log.Logger

// ---------- 按大小轮转的 Writer ----------

type rotatingWriter struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	file    *os.File
	size    int64
}

func newRotatingWriter(path string, maxSize int64, backups int) (*rotatingWriter, error) {
	w := &rotatingWriter{path: path, maxSize: maxSize, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) rotate() error {
	w.file.Close()
	// access.log.2 <- access.log.1 <- access.log
	for i := w.backups - 1; i >= 1; i-- {
		src := w.path + "." + strconv.Itoa(i)
		dst := w.path + "." + strconv.Itoa(i+1)
		os.Rename(src, dst)
	}
	os.Rename(w.path, w.path+".1")
	return w.open()
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			// 轮转失败不能影响服务，退回标准 log（进 app.log）
			log.Printf("access log rotate failed: %v", err)
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// initAccessLog 打开访问日志。失败不影响主服务（退回写到 app.log）。
func initAccessLog() {
	rw, err := newRotatingWriter(accessLogFile, accessLogMaxSize, accessLogBackups)
	if err != nil {
		log.Printf("Failed to open access log %s: %v (access logs will go to app.log)", accessLogFile, err)
		accessLogger = log.New(log.Writer(), "", log.LstdFlags)
		return
	}
	// 不带日期前缀：日期由每条日志自行带，方便按行切分
	accessLogger = log.New(rw, "", 0)
}

// ---------- 状态码/字节数记录器 ----------

type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.status = http.StatusOK
		s.wroteHeader = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// 透传 Flush / Hijack，避免包装掉底层能力（流式响应、连接劫持）
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// clientIP 取客户端地址：有反代头优先，否则用 RemoteAddr
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// accessLog 访问日志中间件：包在整个 ServeMux 外面
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accessLogger == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		accessLogger.Printf("%s %s \"%s\" %s -> %d %dB %s auth=%t len=%d ref=%q ua=%q",
			time.Now().Format("2006/01/02 15:04:05"),
			clientIP(r),
			r.Method+" "+r.URL.RequestURI(),
			r.Proto,
			rec.status,
			rec.bytes,
			time.Since(start).Round(time.Millisecond),
			isAuthorized(r),
			r.ContentLength,
			r.Referer(),
			r.UserAgent(),
		)
	})
}
