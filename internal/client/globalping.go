package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	ProbeID string          `json:"probeId"`
	Result  json.RawMessage `json:"result"`
}
type tracerouteResult struct {
	Hops []trHop `json:"hops"`
}
type trHop struct {
	Timings []trTiming `json:"timings"`
}
type trTiming struct {
	RTT float64 `json:"rtt"`
}

func ClientGlobalping(country, region, city string) (string, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return tracerouteTCP(ctx, country, region, city)
}

func tracerouteTCP(ctx context.Context, country, region, city string) (string, float64, error) {
	if city != "" {
		if id, apiErr, err := postOnce(ctx, &location{City: city}); err != nil {
			return "", 0, err
		} else if apiErr == nil {
			return waitAndExtractRTT(ctx, id)
		} else if apiErr.Error.Type != "no_probes_found" {
			return "", 0, fmt.Errorf("%s: %s", apiErr.Error.Type, apiErr.Error.Message)
		}
	}

	if region != "" {
		if id, apiErr, err := postOnce(ctx, &location{Region: region}); err != nil {
			return "", 0, err
		} else if apiErr == nil {
			return waitAndExtractRTT(ctx, id)
		} else if apiErr.Error.Type != "no_probes_found" {
			return "", 0, fmt.Errorf("%s: %s", apiErr.Error.Type, apiErr.Error.Message)
		}
	}

	if country != "" {
		if id, apiErr, err := postOnce(ctx, &location{Country: country}); err != nil {
			return "", 0, err
		} else if apiErr == nil {
			return waitAndExtractRTT(ctx, id)
		} else if apiErr.Error.Type != "no_probes_found" {
			return "", 0, fmt.Errorf("%s: %s", apiErr.Error.Message, apiErr.Error.Type)
		}
	}

	return "", 0, fmt.Errorf("no_probes_found at all levels (city/region/country)")
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
func waitAndExtractRTT(ctx context.Context, id string) (string, float64, error) {
	deadline, _ := ctx.Deadline()
	backoff := 200 * time.Millisecond
	url := fmt.Sprintf("https://api.globalping.io/v1/measurements/%s", id)

	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", 0, fmt.Errorf("get measurement: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", 0, fmt.Errorf("status %s: %s", resp.Status, string(body))
		}

		var m measurementGetResp
		if err := json.Unmarshal(body, &m); err != nil {
			return "", 0, fmt.Errorf("decode: %w", err)
		}

		switch m.Status {
		case "finished":
			rtts := make([]float64, 0, len(m.Results))
			for _, pr := range m.Results {
				var tr tracerouteResult
				if err := json.Unmarshal(pr.Result, &tr); err != nil || len(tr.Hops) == 0 {
					continue
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
				if n > 0 {
					rtts = append(rtts, sum/float64(n))
				}
			}
			if len(rtts) == 0 {
				return "", 0, fmt.Errorf("finished but no rtt values")
			}

			sort.Float64s(rtts)
			median := rtts[len(rtts)/2]
			return id, median, nil

		case "error":
			if m.Error != nil {
				return "", 0, fmt.Errorf("%s: %s", m.Error.Error.Type, m.Error.Error.Message)
			}
			return "", 0, fmt.Errorf("measurement error")

		default:
			if time.Now().Add(backoff).After(deadline) {
				return "", 0, fmt.Errorf("timeout waiting for measurement %s", id)
			}
			time.Sleep(backoff)
			if backoff < time.Second {
				backoff += 150 * time.Millisecond
			}
		}
	}
}
