package main

import (
	"bytes"
	"cloudease-lite/agent/management"
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// A claimed task is executed once, then its result is retried without re-executing it.
// Tasks interrupted by a restart expire on the server and are never leased again.
func managementLoop(ctx context.Context, client *http.Client, origin, key string, m *management.Manager) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var pending *shared.ManagementResult
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if pending != nil {
			body, _ := json.Marshal(pending)
			req, _ := http.NewRequestWithContext(ctx, "POST", origin+"/api/agent/operations/result", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+key)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
			resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 409 || resp.StatusCode == 410 || resp.StatusCode == 401 || resp.StatusCode == 404 {
				pending = nil
			}
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", origin+"/api/agent/operations/next", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
			resp.Body.Close()
			continue
		}
		var task shared.ManagementTask
		err = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&task)
		resp.Body.Close()
		if err != nil || !shared.ValidKey(task.ID) || !shared.ValidKey(task.Lease) {
			continue
		}
		result := shared.ManagementResult{ID: task.ID, Lease: task.Lease}
		if task.ExpiresAt <= time.Now().UnixMilli() {
			result.Error = "操作已过期"
		} else {
			taskCtx, cancel := context.WithDeadline(ctx, time.UnixMilli(task.ExpiresAt))
			data, err := m.Execute(taskCtx, task.Request)
			cancel()
			if err != nil {
				result.Error = shared.RedactOutput(strings.ReplaceAll(err.Error(), key, "[已隐藏]"))
			} else {
				raw, e := json.Marshal(data)
				if e != nil || len(raw) > 48000 {
					result.Error = "操作返回数据过大"
				} else {
					result.Success = true
					if strings.HasPrefix(task.Request.Action, "file.") {
						result.Data = raw
					} else {
						// Output may contain the client's own key; preserve JSON structure.
						result.Data = json.RawMessage(strings.ReplaceAll(string(raw), key, "[已隐藏]"))
					}
				}
			}
		}
		pending = &result
		log.Printf("Management operation completed: %s success=%t", task.Request.Action, result.Success)
	}
}
