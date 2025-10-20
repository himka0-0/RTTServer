package main

import (
	"RTTServer/internal/client"
	"RTTServer/internal/tcp"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	tcpListenAddr  = ":9000"
	httpListenAddr = ":9080"
	cacheTTL       = time.Hour
	cleanEvery     = 5 * time.Minute
	ioTimeout      = 3 * time.Second
)

type RTTRecord struct {
	IP            string    `json:"ip"`
	TCPI_RTT_us   uint32    `json:"tcpi_rtt_us"`
	RTT_ms        float64   `json:"tcpi_rtt_ms"`
	TCPI_VAR_us   uint32    `json:"tcpi_rttvar_us"`
	RTTVar_ms     float64   `json:"tcpi_rttvar_ms"`
	IDProbeGlabal string    `json:"id_probe_globalping,omitempty"`
	GlobalpingRTT float64   `json:"globalping_rtt_ms,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type cacheStore struct {
	mu   sync.RWMutex
	data map[string]RTTRecord
}

func newCache() *cacheStore { return &cacheStore{data: make(map[string]RTTRecord)} }

func (c *cacheStore) set(rec RTTRecord) {
	c.mu.Lock()
	c.data[rec.IP] = rec
	c.mu.Unlock()
}

func (c *cacheStore) get(ip string) (RTTRecord, bool) {
	c.mu.RLock()
	rec, ok := c.data[ip]
	c.mu.RUnlock()
	if !ok || time.Since(rec.UpdatedAt) > cacheTTL {
		return RTTRecord{}, false
	}
	return rec, true
}

func (c *cacheStore) allFresh() []RTTRecord {
	now := time.Now()
	c.mu.RLock()
	out := make([]RTTRecord, 0, len(c.data))
	for _, r := range c.data {
		if now.Sub(r.UpdatedAt) <= cacheTTL {
			out = append(out, r)
		}
	}
	c.mu.RUnlock()
	return out
}

func (c *cacheStore) janitor() {
	t := time.NewTicker(cleanEvery)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		c.mu.Lock()
		for k, v := range c.data {
			if now.Sub(v.UpdatedAt) > cacheTTL {
				delete(c.data, k)
			}
		}
		c.mu.Unlock()
	}
}

func main() {
	cache := newCache()
	go cache.janitor()

	mux := http.NewServeMux()
	mux.HandleFunc("/rtt", func(w http.ResponseWriter, r *http.Request) {
		ip := strings.TrimSpace(r.URL.Query().Get("ip"))
		if ip == "" {
			http.Error(w, "use /rtt?ip=1.2.3.4 or /rtt/all", http.StatusBadRequest)
			return
		}
		rec, ok := cache.get(ip)
		if !ok {
			http.Error(w, "not found or expired", http.StatusNotFound)
			return
		}
		writeJSON(w, rec)
	})
	mux.HandleFunc("/rtt/all", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, cache.allFresh())
	})
	go func() {
		log.Printf("HTTP listening on %s", httpListenAddr)
		if err := http.ListenAndServe(httpListenAddr, logRequest(mux)); err != nil {
			log.Fatalf("http serve: %v", err)
		}
	}()

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
		go handleConn(c, cache)
	}
}

func handleConn(c net.Conn, cache *cacheStore) {
	defer c.Close()

	remoteIP := peerIP(c.RemoteAddr())
	if remoteIP == "" {
		return
	}

	_ = c.SetDeadline(time.Now().Add(ioTimeout))
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	buf := make([]byte, 1)
	_, _ = c.Read(buf)
	_, _ = c.Write([]byte{1})
	_ = c.SetDeadline(time.Time{})
	var (
		rttUS, rttVarUS uint32
		err             error
	)
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		rttUS, rttVarUS, err = tcp.TcpInfoRTT(c)
		if err == nil && (rttUS != 0 || rttVarUS != 0) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		log.Printf("tcp_info %s: %v", remoteIP, err)
		return
	}
	country, region, city, err := client.ClientIPAPI(remoteIP)
	if err != nil {
		log.Printf("ip-api %s: %v", remoteIP, err)
	}
	id, gpRTTms, err := client.ClientGlobalping(country, region, city)
	if err != nil {
		log.Printf("globalping %s: %v", remoteIP, err)
	}
	rec := RTTRecord{
		IP:            remoteIP,
		TCPI_RTT_us:   rttUS,
		RTT_ms:        float64(rttUS) / 1000.0,
		TCPI_VAR_us:   rttVarUS,
		RTTVar_ms:     float64(rttVarUS) / 1000.0,
		IDProbeGlabal: id,
		GlobalpingRTT: gpRTTms,
		UpdatedAt:     time.Now(),
	}
	cache.set(rec)
	log.Printf("updated ip=%s rtt=%.3fms var=%.3fms", rec.IP, rec.RTT_ms, rec.RTTVar_ms)
}

func peerIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	return host
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
