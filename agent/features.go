package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// v0.1 rejects unknown report fields, so authenticate feature negotiation first.
func supportsExtensions(ctx context.Context, client *http.Client, origin, key string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", origin+"/api/agent/check", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return false
	}
	var features struct {
		ManagementProtocol int `json:"management_protocol"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&features) != nil {
		return false
	}
	return features.ManagementProtocol == 1
}
