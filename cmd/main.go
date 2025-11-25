package main

import (
	"RTTServer/internal/cache"
	"RTTServer/internal/echo"
	"RTTServer/internal/tcp"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	tcpListenAddr  = ":9000"
	httpListenAddr = ":9080"
	cleanEvery     = 5 * time.Minute
)

func main() {
	store := cache.New()
	go store.Janitor(cleanEvery)

	mux := http.NewServeMux()
	mux.HandleFunc("/rtt", func(w http.ResponseWriter, r *http.Request) {
		ip := strings.TrimSpace(r.URL.Query().Get("ip"))
		if ip == "" {
			http.Error(w, "use /rtt?ip=1.2.3.4 or /rtt/all", http.StatusBadRequest)
			return
		}
		rec, ok := store.Get(ip)
		if !ok {
			http.Error(w, "not found or expired", http.StatusNotFound)
			return
		}
		writeJSON(w, rec)
	})
	mux.HandleFunc("/rtt/all", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, store.AllFresh())
	})
	go func() {
		log.Printf("HTTP listening on %s", httpListenAddr)
		if err := http.ListenAndServe(httpListenAddr, logRequest(mux)); err != nil {
			log.Fatalf("http serve: %v", err)
		}
	}()

	//echo порты
	go echo.StartEchoFiltered(":8443", 2*time.Second, func(c net.Conn) { tcp.HandleConn(c, store) })
	go echo.StartEchoFiltered(":8080", 2*time.Second, func(c net.Conn) { tcp.HandleConn(c, store) })
	go echo.StartEchoFiltered(":853", 2*time.Second, func(c net.Conn) { tcp.HandleConn(c, store) })
	go echo.StartEchoFiltered(":5432", 2*time.Second, func(c net.Conn) { tcp.HandleConn(c, store) })
	ln, err := net.Listen("tcp", tcpListenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", tcpListenAddr, err)
	}
	log.Printf("TCP listening on %s", tcpListenAddr)

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go tcp.HandleConn(c, store)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
