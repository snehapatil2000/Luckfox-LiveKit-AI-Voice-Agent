package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowedAutomationRequests(t *testing.T) {
	tests := []struct {
		method  string
		path    string
		allowed bool
	}{
		{"GET", "/api/v1/points", true},
		{"GET", "/api/v1/rooms", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/on", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/set", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/fan/speed/up", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/fan/speed/down", true},
		{"POST", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11/fan/speed/set", true},
		{"POST", "/api/v1/scenes/7/activate", true},
		{"POST", "/api/v1/moods/9/activate", true},
		{"DELETE", "/api/v1/points/40:4c:ca:ff:fe:5a:2b:10:11", false},
		{"POST", "/api/v1/users/1/delete", false},
		{"POST", "/api/v1/points/../../users/on", false},
		{"GET", "/api/v1/auth/tokens", false},
	}
	for _, test := range tests {
		if got := isAllowedAutomationRequest(test.method, test.path); got != test.allowed {
			t.Errorf("isAllowedAutomationRequest(%q, %q) = %v, want %v", test.method, test.path, got, test.allowed)
		}
	}
}

func TestAutomationBridgeLogsInAndRetriesExpiredToken(t *testing.T) {
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"access_token": "fresh-token"}})
		case "/api/v1/points":
			if r.Header.Get("Authorization") == "Bearer expired-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer fresh-token" {
				t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"points":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	bridge := &automationBridge{
		baseURL:  server.URL,
		token:    "expired-token",
		username: "voice_agent",
		password: "secret",
		client:   server.Client(),
	}
	status, _, err := bridge.request(geckoLinkRequest{Method: http.MethodGet, Path: "/api/v1/points"})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || loginCount != 1 {
		t.Fatalf("status=%d loginCount=%d, want 200 and 1", status, loginCount)
	}
}
