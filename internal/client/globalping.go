package client

import (
	"RTTServer/internal/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type location struct {
	Country string `json:"country,omitempty"`
	Region  string `json:"magic,omitempty"`
	City    string `json:"city,omitempty"`
}
type measurementOptions struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

type measurementReq struct {
	Target             string             `json:"target"`
	Type               string             `json:"type"`
	Locations          []location         `json:"locations,omitempty"`
	MeasurementOptions measurementOptions `json:"measurementOptions"`
	Limit              int                `json:"limit,omitempty"`
}

type okResp struct {
	ID          string `json:"id"`
	ProbesCount int    `json:"probesCount"`
}

type errResp struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type measurementGetResp struct {
	Status  string        `json:"status"`
	Results []probeResult `json:"results"`
	Error   *errResp      `json:"error,omitempty"`
}

type probeResult struct {
	Probe  probeMeta       `json:"probe"`
	Result json.RawMessage `json:"result"`
}

type probeMeta struct {
	Continent string   `json:"continent"`
	Country   string   `json:"country"`
	State     string   `json:"state"`
	City      string   `json:"city"`
	ASN       int      `json:"asn"`
	Network   string   `json:"network"`
	Longitude float64  `json:"longitude"`
	Latitude  float64  `json:"latitude"`
	Tags      []string `json:"tags"`
}

type ProbeInfo struct {
	IP        *string `json:"ip,omitempty"`
	RTTms     float64 `json:"rtt_ms"`
	Longitude float64 `json:"longitude,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	ASN       int     `json:"asn,omitempty"`
	Network   string  `json:"network,omitempty"`
	Country   string  `json:"country,omitempty"`
	City      string  `json:"city,omitempty"`
	Distance  float64 `json:"distance_km,omitempty"`
	HopCount  int     `json:"hop_count,omitempty"`
}

type GlobalpingAgg struct {
	MeasurementID string      `json:"id_probe_globalping"`
	RTTMedianMS   float64     `json:"globalping_rtt_ms"`
	Probes        []ProbeInfo `json:"info_probes"`
}
type trHop struct {
	Timings []trTiming `json:"timings"`
}
type trTiming struct {
	RTT float64 `json:"rtt"`
}

func ClientGlobalping(country, region, city string) (GlobalpingAgg, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return tracerouteTCP(ctx, country, region, city)
}

func tracerouteTCP(ctx context.Context, country, region, city string) (GlobalpingAgg, error) {
	fmt.Println(country, region, city)
	if city != "" {
		if id, apiErr, err := postOnce(ctx, &location{City: city}); err != nil {
			return GlobalpingAgg{}, err
		} else if apiErr == nil {
			return waitAndExtractAgg(ctx, id)
		} else if apiErr.Error.Type != "no_probes_found" {
			return GlobalpingAgg{}, fmt.Errorf("%s: %s", apiErr.Error.Type, apiErr.Error.Message)
		}
	}

	if region != "" {
		if id, apiErr, err := postOnce(ctx, &location{Region: region}); err != nil {
			return GlobalpingAgg{}, err
		} else if apiErr == nil {
			return waitAndExtractAgg(ctx, id)
		} else if apiErr.Error.Type != "no_probes_found" {
			return GlobalpingAgg{}, fmt.Errorf("%s: %s", apiErr.Error.Type, apiErr.Error.Message)
		}
	}

	if country != "" {
		if id, apiErr, err := postOnce(ctx, &location{Country: country}); err != nil {
			return GlobalpingAgg{}, err
		} else if apiErr == nil {
			return waitAndExtractAgg(ctx, id)
		} else if apiErr.Error.Type != "no_probes_found" {
			return GlobalpingAgg{}, fmt.Errorf("%s: %s", apiErr.Error.Message, apiErr.Error.Type)
		}
	}

	return GlobalpingAgg{}, fmt.Errorf("no_probes_found at all levels (city/region/country)")
}

func postOnce(ctx context.Context, loc *location) (string, *errResp, error) {
	reqBody := measurementReq{
		Target: "45.61.141.20",
		Type:   "traceroute",
		MeasurementOptions: measurementOptions{
			Protocol: "tcp",
			Port:     9000,
		},
		Limit: 3,
	}
	if loc != nil {
		reqBody.Locations = []location{*loc}
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("marshal: %w")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.globalping.io/v1/measurements", bytes.NewReader(payload))
	if err != nil {
		return "", nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var ok okResp
		if err := json.Unmarshal(body, &ok); err == nil && ok.ID != "" {
			return ok.ID, nil, nil
		}
		return "", nil, fmt.Errorf("unexpected 2xx schema: %s", string(body))
	}

	var e errResp
	if err := json.Unmarshal(body, &e); err == nil && e.Error.Type != "" {
		return "", &e, nil
	}

	return "", nil, fmt.Errorf("status %s: %s", resp.Status, string(body))
}

func waitAndExtractAgg(ctx context.Context, id string) (GlobalpingAgg, error) {
	url := fmt.Sprintf("https://api.globalping.io/v1/measurements/%s", id)
	backoff := 200 * time.Millisecond
	deadline, _ := ctx.Deadline()

	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return GlobalpingAgg{}, fmt.Errorf("get measurement: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return GlobalpingAgg{}, fmt.Errorf("status %s: %s", resp.Status, string(body))
		}

		var m measurementGetResp
		if err := json.Unmarshal(body, &m); err != nil {
			return GlobalpingAgg{}, fmt.Errorf("decode: %w", err)
		}

		switch m.Status {
		case "finished":
			rtts := make([]float64, 0, len(m.Results))
			infos := make([]ProbeInfo, 0, len(m.Results))

			for _, re := range m.Results {
				var tr struct {
					RawOutput        string `json:"rawOutput"`
					ResolvedAddress  string `json:"resolvedAddress"`
					ResolvedHostname string `json:"resolvedHostname"`
					Hops             []struct {
						ResolvedHostname string `json:"resolvedHostname"`
						ResolvedAddress  string `json:"resolvedAddress"`
						Timings          []struct {
							RTT float64 `json:"rtt"`
						} `json:"timings"`
					} `json:"hops"`
				}
				if err := json.Unmarshal(re.Result, &tr); err != nil || len(tr.Hops) == 0 {
					continue
				}

				hopCount := 0
				targetIP := strings.TrimSpace(tr.ResolvedAddress)
				targetHost := strings.TrimSpace(tr.ResolvedHostname)
				for i, h := range tr.Hops {
					if (targetIP != "" && strings.EqualFold(h.ResolvedAddress, targetIP)) ||
						(targetHost != "" && strings.EqualFold(h.ResolvedHostname, targetHost)) {
						hopCount = i + 1
						break
					}
				}

				if hopCount == 0 && tr.RawOutput != "" && (targetIP != "" || targetHost != "") {
					hopNumRe := regexp.MustCompile(`^\s*(\d+)\s+`)
					for _, ln := range strings.Split(tr.RawOutput, "\n") {
						low := strings.ToLower(ln)
						if (targetIP != "" && strings.Contains(low, strings.ToLower(targetIP))) ||
							(targetHost != "" && strings.Contains(low, strings.ToLower(targetHost))) {
							if m := hopNumRe.FindStringSubmatch(ln); len(m) == 2 {
								if n, err := strconv.Atoi(m[1]); err == nil {
									hopCount = n
									break
								}
							}
						}
					}
				}
				last := tr.Hops[len(tr.Hops)-1]
				var sum float64
				var n int
				for _, t := range last.Timings {
					if t.RTT > 0 {
						sum += t.RTT
						n++
					}
				}
				if n == 0 {
					continue
				}
				avg := sum / float64(n)
				rtts = append(rtts, avg)
				distance := utils.Haversine(36.102, -115.1447, re.Probe.Latitude, re.Probe.Longitude)
				infos = append(infos, ProbeInfo{
					IP:        nil,
					RTTms:     avg,
					Longitude: re.Probe.Longitude,
					Latitude:  re.Probe.Latitude,
					ASN:       re.Probe.ASN,
					Network:   re.Probe.Network,
					Country:   re.Probe.Country,
					City:      re.Probe.City,
					Distance:  distance,
					HopCount:  hopCount,
				})
			}

			if len(rtts) == 0 {
				return GlobalpingAgg{}, fmt.Errorf("finished but no rtt values")
			}
			sort.Float64s(rtts)
			median := rtts[len(rtts)/2]

			return GlobalpingAgg{
				MeasurementID: id,
				RTTMedianMS:   median,
				Probes:        infos,
			}, nil

		case "error":
			return GlobalpingAgg{}, fmt.Errorf("measurement error")

		default:
			if time.Now().Add(backoff).After(deadline) {
				return GlobalpingAgg{}, fmt.Errorf("timeout waiting for measurement %s", id)
			}
			time.Sleep(backoff)
			if backoff < time.Second {
				backoff += 150 * time.Millisecond
			}
		}
	}
}
