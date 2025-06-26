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
	log.Printf("WS request to: %s\n", r.URL.Path)

	// Kết nối đến WebSocket backend
	backendConn, err := net.Dial("tcp", "localhost:8003")
	if err != nil {
		http.Error(w, "WebSocket backend unavailable", http.StatusBadGateway)
		log.Println("Dial error:", err)
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
		log.Println("Error forwarding request:", err)
		return
	}

	// Gửi và nhận dữ liệu WebSocket
	go io.Copy(backendConn, clientConn)
	io.Copy(clientConn, backendConn)
}

func main() {
	// HTTP reverse proxy
	http.HandleFunc("/stock/", reverseProxy("http://localhost:8001"))
	http.HandleFunc("/service-b/", reverseProxy("http://localhost:8002"))

	// WebSocket proxy
	http.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
			strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			proxyWebSocket(w, r)
		} else {
			http.Error(w, "Not a WebSocket request", http.StatusBadRequest)
		}
	})

	log.Println("API Gateway chạy tại http://localhost:80")
	log.Fatal(http.ListenAndServe(":80", nil))
}
