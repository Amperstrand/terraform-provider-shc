package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientOptions_whenMaxRetriesZero_disablesRetries(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewSHCClientWithOptions("test-key", server.URL, ClientOptions{MaxRetries: -1})
	_, _, err := client.doRequest(context.Background(), http.MethodGet, "/vm", nil, "")
	if err == nil {
		t.Fatal("expected error from 503 response")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("expected exactly 1 attempt with retries disabled, got %d", got)
	}
}

func TestClientOptions_whenDefaultRetries_retriesRetryableStatus(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, _, _ = client.doRequest(context.Background(), http.MethodGet, "/vm", nil, "")
	if got := attempts.Load(); got != 1+int64(httpMaxRetries) {
		t.Errorf("expected %d attempts (1 + %d retries), got %d", 1+httpMaxRetries, httpMaxRetries, got)
	}
}

func TestClientOptions_whenTimeout_shortDeadlineFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": {}}`))
	}))
	defer server.Close()

	client := NewSHCClientWithOptions("test-key", server.URL, ClientOptions{Timeout: 100 * time.Millisecond})
	start := time.Now()
	_, _, err := client.doRequest(context.Background(), http.MethodGet, "/vm", nil, "")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 290*time.Millisecond {
		t.Errorf("client waited %v despite a 100ms timeout", elapsed)
	}
}

func TestClientOptions_whenRateLimit_throttlesRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": {"items": [], "pagination": {}}}`))
	}))
	defer server.Close()

	// 20 rps with burst 1: the 3rd+ requests must wait >= ~100ms in total.
	client := NewSHCClientWithOptions("test-key", server.URL, ClientOptions{RateLimitRPS: 20})
	start := time.Now()
	for range 3 {
		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/vm", nil, "")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Errorf("3 requests at 20rps completed in %v — limiter not applied", elapsed)
	}
}

func TestClientOptions_defaultsPreserved(t *testing.T) {
	// Zero options must behave exactly like NewSHCClient: 60s timeout is
	// internal, so pin the observable contract via JSON decode of a normal
	// request through both constructors.
	var seenPaths atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths.Add(1)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}}})
	}))
	defer server.Close()

	for _, client := range []*SHCClient{
		NewSHCClient("k", server.URL),
		NewSHCClientWithOptions("k", server.URL, ClientOptions{}),
	} {
		if _, _, err := client.doRequest(context.Background(), http.MethodGet, "/vm", nil, ""); err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}
	if got := seenPaths.Load(); got != 2 {
		t.Errorf("expected 2 successful requests, got %d", got)
	}
}
