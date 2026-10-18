package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatedVersionNegotiation(t *testing.T) {
	for _, tc := range []struct {
		body     string
		status   int
		expected bool
	}{{`{"ok":true}`, 200, false}, {`{"ok":true,"management_protocol":1}`, 200, true}, {`{"management_protocol":1}`, 401, false}, {`{"management_protocol":999}`, 200, false}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/agent/check" || r.Header.Get("Authorization") != "Bearer test-key" {
				t.Error("feature check not authenticated")
			}
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		got := supportsExtensions(context.Background(), server.Client(), server.URL, "test-key")
		server.Close()
		if got != tc.expected {
			t.Fatalf("features %s: %t", tc.body, got)
		}
	}
}
func TestWebsiteProtocolNegotiation(t *testing.T) {
	for _, tc := range []struct {
		body     string
		expected bool
	}{{`{"management_protocol":1}`, false}, {`{"website_protocol":1}`, true}, {`{"website_protocol":2}`, false}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Error("unauthenticated website protocol")
			}
			w.Write([]byte(tc.body))
		}))
		got := supportsWebsites(context.Background(), server.Client(), server.URL, "test-key")
		server.Close()
		if got != tc.expected {
			t.Fatal(tc.body, got)
		}
	}
}
