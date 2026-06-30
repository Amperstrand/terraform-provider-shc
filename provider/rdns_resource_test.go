package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetReverseDNS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header 'Bearer test-key', got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/vm/123/rdns" {
			t.Errorf("expected path /vm/123/rdns, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]string
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		if reqBody["ip"] != "1.2.3.4" {
			t.Errorf("expected ip 1.2.3.4, got %s", reqBody["ip"])
		}
		if reqBody["hostname"] != "host.example.com" {
			t.Errorf("expected hostname host.example.com, got %s", reqBody["hostname"])
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"job_id": "job-abc-123",
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	resp, err := client.SetReverseDNS(context.Background(), "123", "1.2.3.4", "host.example.com")
	if err != nil {
		t.Fatalf("SetReverseDNS failed: %v", err)
	}
	if resp.JobID.String() != "job-abc-123" {
		t.Errorf("expected job_id job-abc-123, got %s", resp.JobID.String())
	}
}

func TestGetReverseDNS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/rdns" {
			t.Errorf("expected path /vm/123/rdns, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"records": []map[string]interface{}{
					{"ip": "1.2.3.4", "hostname": "host.example.com"},
					{"ip": "5.6.7.8", "hostname": "other.example.com"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	records, err := client.GetReverseDNS(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetReverseDNS failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].IP != "1.2.3.4" {
		t.Errorf("expected first IP 1.2.3.4, got %s", records[0].IP)
	}
	if records[0].Hostname != "host.example.com" {
		t.Errorf("expected first hostname host.example.com, got %s", records[0].Hostname)
	}
	if records[1].IP != "5.6.7.8" {
		t.Errorf("expected second IP 5.6.7.8, got %s", records[1].IP)
	}
}

func TestGetReverseDNS_ArrayFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"ip": "10.0.0.1", "hostname": "api.example.com"},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	records, err := client.GetReverseDNS(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetReverseDNS failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].IP != "10.0.0.1" {
		t.Errorf("expected IP 10.0.0.1, got %s", records[0].IP)
	}
	if records[0].Hostname != "api.example.com" {
		t.Errorf("expected hostname api.example.com, got %s", records[0].Hostname)
	}
}

func TestClearReverseDNS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/rdns" {
			t.Errorf("expected path /vm/123/rdns, got %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]string
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		if reqBody["ip"] != "1.2.3.4" {
			t.Errorf("expected ip 1.2.3.4, got %s", reqBody["ip"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.ClearReverseDNS(context.Background(), "123", "1.2.3.4")
	if err != nil {
		t.Fatalf("ClearReverseDNS failed: %v", err)
	}
}

func TestClearReverseDNS_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.ClearReverseDNS(context.Background(), "123", "1.2.3.4")
	if err != nil {
		t.Fatalf("ClearReverseDNS should return nil on 404, got: %v", err)
	}
}

func TestSetPowerState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/stop" {
			t.Errorf("expected path /vm/123/stop, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.SetPowerState(context.Background(), "123", "stop")
	if err != nil {
		t.Fatalf("SetPowerState failed: %v", err)
	}
}

func TestSetPowerState_Start(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/start" {
			t.Errorf("expected path /vm/123/start, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.SetPowerState(context.Background(), "123", "start")
	if err != nil {
		t.Fatalf("SetPowerState failed: %v", err)
	}
}
