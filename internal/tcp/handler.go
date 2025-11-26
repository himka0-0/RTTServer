package tcp

import (
	"RTTServer/internal/cache"
	"RTTServer/internal/client"
	"RTTServer/internal/model"
	"RTTServer/internal/utils"
	"log"
	"net"
	"sync"
	"time"
)

const (
	ioTimeout       = 3 * time.Second
	globalpingIPTTL = 10 * time.Minute
)

func HandleConn(c net.Conn, store *cache.Store) {
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
		rttUS, rttVarUS, err = TcpInfoRTT(c)
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
	country, region, city, lat, lon, err := client.ClientIPAPI(remoteIP)
	if err != nil {
		log.Printf("ip-api %s: %v", remoteIP, err)
	}

	distanceToServer := utils.Haversine(36.102, -115.1447, lat, lon)

	prev, hasPrev := store.Get(remoteIP)
	var agg client.GlobalpingAgg

	if gpGate.Allow(remoteIP, globalpingIPTTL) {
		res, err := client.ClientGlobalping(country, region, city)
		if err != nil {
			log.Printf("globalping %s: %v", remoteIP, err)
			if hasPrev {
				agg.MeasurementID = prev.IDProbeGlabal
				agg.RTTMedianMS = prev.GlobalpingRTT
				agg.Probes = prev.InfoProbes
				agg.RawOutputs = prev.Rawdate
			}
		} else {
			agg = res
		}
	} else if hasPrev {
		agg.MeasurementID = prev.IDProbeGlabal
		agg.RTTMedianMS = prev.GlobalpingRTT
		agg.Probes = prev.InfoProbes
		agg.RawOutputs = prev.Rawdate
	}

	rec := model.RTTRecord{
		IP:               remoteIP,
		DistanceToServer: distanceToServer,
		TCPI_RTT_us:      rttUS,
		RTT_ms:           float64(rttUS) / 1000.0,
		TCPI_VAR_us:      rttVarUS,
		RTTVar_ms:        float64(rttVarUS) / 1000.0,
		IDProbeGlabal:    agg.MeasurementID,
		GlobalpingRTT:    agg.RTTMedianMS,
		InfoProbes:       agg.Probes,
		UpdatedAt:        time.Now(),
		Rawdate:          agg.RawOutputs,
	}
	store.Set(rec)
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

// чтоб на global отправлялся ip только один раз

type ipGate struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newIPGate() *ipGate { return &ipGate{seen: make(map[string]time.Time)} }

func (g *ipGate) Allow(ip string, ttl time.Duration) bool {
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if t, ok := g.seen[ip]; ok && now.Sub(t) < ttl {
		return false
	}
	g.seen[ip] = now
	return true
}

var gpGate = newIPGate()
