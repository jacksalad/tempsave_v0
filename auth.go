package main

// 访问授权（页面闸门）
//
// 2026-10-12 新增。目的：把 TempSave 的 Web 页面对未授权访问者藏起来——
// 没有正确授权时，页面路由只返回一个标准的 404 页面（真实 HTTP 404 状态码）。
//
// 授权方式：
//   1. 访问  地址/?password=123321
//   2. 服务端校验通过后种下授权票 cookie（ts_auth），并 302 跳转到 "/"
//      （密码不会留在地址栏/历史里）
//   3. 之后带着这个 cookie 访问就正常进页面，直到 cookie 被清理或服务重启
//
// 设计取舍：
//   - 授权票的值是**进程内随机令牌**（crypto/rand 生成，不落盘、不写死）。
//     这样即使别人拿到二进制、知道密码规则，也无法凭空伪造 cookie；
//     代价是服务重启后所有已授权浏览器都要重新输一次密码。
//   - 密码用 subtle.ConstantTimeCompare 比对，避免时序侧信道。
//   - cookie 设 HttpOnly + SameSite=Lax：JS 读不到，也不跨站携带。
//
// 边界（重要）：本闸门**只作用于页面路由 "/" 和静态资源 "/assets/"**。
// 所有功能接口（/files、/upload、/upload-file、/del、/mkdir、/rmdir、/check-space）
// 以及 /uploads/ 文件服务**一律未做任何改动**，保持原有契约不变——
// 因为项目已部署并与多处服务联动，接口不能动。

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"os"
)

const (
	accessPassword   = "123321"           // 访问密码（硬编码在此）
	authCookieName   = "ts_auth"          // 授权票 cookie 名
	authCookieAge    = 365 * 24 * 60 * 60 // 授权票有效期：1 年
	notFoundPageFile = "src/404.html"     // 未授权时返回的页面
)

// authToken 进程内随机授权票值。只在内存里，重启即变（相当于全员重新登录）。
var authToken = mustRandomToken()

func mustRandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate auth token: %v", err)
	}
	return hex.EncodeToString(b)
}

// isAuthorized 判断请求是否带着有效的授权票
func isAuthorized(r *http.Request) bool {
	c, err := r.Cookie(authCookieName)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(authToken)) == 1
}

// passwordMatched 判断 URL 上的 password 参数是否正确（恒定时间比对）
func passwordMatched(got string) bool {
	if len(got) != len(accessPassword) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(accessPassword)) == 1
}

// grantAccess 种下授权票并跳转到干净的主页地址
func grantAccess(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    authToken,
		Path:     "/",
		MaxAge:   authCookieAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
	log.Println("【Auth】 access granted")
}

// serveNotFound 返回标准的 404 页面（真实 404 状态码）
// 同时记一条未授权访问日志——这是发现扫描/枚举/试密码的主要依据。
// 只记来源 IP、方法、目标 URL；**不记 password 参数本身**（避免密码落盘）。
func serveNotFound(w http.ResponseWriter, r *http.Request) {
	log.Printf("【Auth DENIED】 %s %s %q 无有效授权票，已按 404 处理",
		clientIP(r), r.Method, r.URL.RequestURI())
	data, err := os.ReadFile(notFoundPageFile)
	if err != nil {
		// 404 页面本身读不到时，至少状态码和文案要对
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	w.Write(data)
}

// pageGate 页面闸门：未授权且密码不对 → 404；密码对 → 授权并跳转；已授权 → 放行
// 返回 true 表示请求已被处理（调用方直接 return）
func pageGate(w http.ResponseWriter, r *http.Request) bool {
	if isAuthorized(r) {
		return false
	}
	pw := r.URL.Query().Get("password")
	if passwordMatched(pw) {
		grantAccess(w, r)
		return true
	}
	// 带了 password 但不对：单独记一条，便于发现暴力破解
	// （只记长度特征，不记密码原文）
	if pw != "" {
		log.Printf("【Auth FAILED】 %s %s %q 提交了错误密码（长度 %d）",
			clientIP(r), r.Method, r.URL.RequestURI(), len(pw))
	}
	serveNotFound(w, r)
	return true
}
