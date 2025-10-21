package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ipAPIResp struct {
	Status     string  `json:"status"`
	Country    string  `json:"country"`
	RegionName string  `json:"regionName"`
	City       string  `json:"city"`
	Message    string  `json:"message"`
	Latitude   float64 `json:"lat"`
	Longitude  float64 `json:"lon"`
}

func ClientIPAPI(remoteIP string) (string, string, string, float64, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return lookupGeo(ctx, remoteIP)
}

func lookupGeo(ctx context.Context, ip string) (country, region, city string, Latitude, Longitude float64, err error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,regionName,city,lat,lon,message", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	var httpClient = &http.Client{Timeout: 3 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0, 0, fmt.Errorf("ip-api http %d", resp.StatusCode)
	}
	var r ipAPIResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", "", 0, 0, err
	}
	if r.Status != "success" {
		if r.Message != "" {
			return "", "", "", 0, 0, fmt.Errorf("ip-api: %s", r.Message)
		}
		return "", "", "", 0, 0, fmt.Errorf("ip-api: failed")
	}
	return r.Country, r.RegionName, r.City, r.Latitude, r.Longitude, nil
}
