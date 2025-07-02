package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Proxy HTTP thông thường
func reverseProxy(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, "Bad target URL", http.StatusInternalServerError)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)

		// Ghi đè Director để chỉnh path
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)

			// Xóa tiền tố "/stock" hoặc "/service-b"
			if strings.HasPrefix(req.URL.Path, "/stock/") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/stock")
			} else if strings.HasPrefix(req.URL.Path, "/service-b/") {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/service-b")
			}
		}

		proxy.ServeHTTP(w, r)
	}
}

// Proxy WebSocket
func proxyWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔄 WS request to: %s\n", r.URL.Path)

	// Kết nối đến WebSocket backend trên port 9999
	backendConn, err := net.Dial("tcp", "localhost:9999")
	if err != nil {
		http.Error(w, "WebSocket backend unavailable", http.StatusBadGateway)
		log.Printf("❌ Dial error: %v\n", err)
		return
	}
	defer backendConn.Close()

	// Hijack kết nối client
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "Hijack failed", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Forward request ban đầu đến backend (bao gồm header WebSocket)
	err = r.Write(backendConn)
	if err != nil {
		log.Printf("❌ Error forwarding request: %v\n", err)
		return
	}

	log.Println("✅ WebSocket connection established")

	// Gửi và nhận dữ liệu WebSocket
	go func() {
		defer func() {
			log.Println("🔚 Client -> Backend connection closed")
		}()
		io.Copy(backendConn, clientConn)
	}()

	defer func() {
		log.Println("🔚 Backend -> Client connection closed")
	}()
	io.Copy(clientConn, backendConn)
}

func main() {
	// HTTP reverse proxy
	http.HandleFunc("/stock/", reverseProxy("http://localhost:8001"))
	http.HandleFunc("/service-b/", reverseProxy("http://localhost:8002"))

	// WebSocket proxy handler
	wsHandler := func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
			strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			proxyWebSocket(w, r)
		} else {
			http.Error(w, "Not a WebSocket request", http.StatusBadRequest)
		}
	}

	// Route cho WebSocket - chỉ cần 1 pattern
	http.HandleFunc("/ws", wsHandler)  // Match chính xác /ws
	http.HandleFunc("/ws/", wsHandler) // Match /ws/ và sub-paths

	log.Println("🚀 API Gateway chạy tại http://localhost:80")
	log.Println("📡 WebSocket proxy: ws://localhost:80/ws -> ws://localhost:9999/ws")
	log.Fatal(http.ListenAndServe(":80", nil))
}
