package main

import (
	"cloudease-lite/agent/management"
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestV05RejectedResultDoesNotBlockAllFutureTasks(t *testing.T) {
	var claims atomic.Int32
	next := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing Agent identity")
		}
		if r.Method == "POST" {
			w.WriteHeader(400)
			return
		}
		if claims.Add(1) > 1 {
			select {
			case next <- struct{}{}:
			default:
			}
			w.WriteHeader(204)
			return
		}
		json.NewEncoder(w).Encode(shared.ManagementTask{ID: strings.Repeat("A", 43), Lease: strings.Repeat("B", 43), ExpiresAt: time.Now().Add(45 * time.Second).UnixMilli(), Request: shared.ManagementRequest{Action: "command.disk"}})
	}))
	defer srv.Close()
	m, err := management.New(false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); managementLoop(ctx, srv.Client(), srv.URL, "fixture", m) }()
	defer func() { cancel(); <-done }()
	select {
	case <-next:
	case <-time.After(9 * time.Second):
		t.Fatal("permanently invalid result retried forever; queue blocked")
	}
}
