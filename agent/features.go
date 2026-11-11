package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// v0.1 rejects unknown report fields, so authenticate feature negotiation first.
type controlFeatures struct {
	ManagementProtocol int `json:"management_protocol"`
	WebsiteProtocol    int `json:"website_protocol"`
	SecurityProtocol   int `json:"security_protocol"`
}

func negotiateFeatures(ctx context.Context, client *http.Client, origin, key string) controlFeatures {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", origin+"/api/agent/check", nil)
	if err != nil {
		return controlFeatures{}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := client.Do(req)
	if err != nil {
		return controlFeatures{}
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return controlFeatures{}
	}
	var features controlFeatures
	if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&features) != nil {
		return controlFeatures{}
	}
	return features
}
func supportsExtensions(ctx context.Context, client *http.Client, origin, key string) bool {
	return negotiateFeatures(ctx, client, origin, key).ManagementProtocol == 1
}
func supportsWebsites(ctx context.Context, client *http.Client, origin, key string) bool {
	return negotiateFeatures(ctx, client, origin, key).WebsiteProtocol == 1
}

func supportsSecurity(ctx context.Context, client *http.Client, origin, key string) bool {
	return negotiateFeatures(ctx, client, origin, key).SecurityProtocol == 1
}
