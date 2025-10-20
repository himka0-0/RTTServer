package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ipAPIResp struct {
	Status     string `json:"status"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
	Message    string `json:"message"`
}

func ClientIPAPI(remoteIP string) (string, string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return lookupGeo(ctx, remoteIP)
}

func lookupGeo(ctx context.Context, ip string) (country, region, city string, err error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,regionName,city,message", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", "", err
	}
	var httpClient = &http.Client{Timeout: 3 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("ip-api http %d", resp.StatusCode)
	}
	var r ipAPIResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", "", err
	}
	if r.Status != "success" {
		if r.Message != "" {
			return "", "", "", fmt.Errorf("ip-api: %s", r.Message)
		}
		return "", "", "", fmt.Errorf("ip-api: failed")
	}
	return r.Country, r.RegionName, r.City, nil
}
