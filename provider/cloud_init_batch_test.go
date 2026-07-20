package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateCloudInit(t *testing.T) {
	var receivedPath, receivedMethod, receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedMethod = r.Method
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{"accepted":true,"lintReport":{"accepted":true}}}`))
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	result, err := client.ValidateCloudInit(t.Context(), "1077", "#cloud-config\npackage_update: true\n")
	if err != nil {
		t.Fatalf("ValidateCloudInit failed: %v", err)
	}
	if receivedPath != "/virtual-machines/1077/cloud-init/validate" {
		t.Errorf("expected path /virtual-machines/1077/cloud-init/validate, got %s", receivedPath)
	}
	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	var body map[string]string
	json.Unmarshal([]byte(receivedBody), &body)
	if body["cloudInit"] != "#cloud-config\npackage_update: true\n" {
		t.Errorf("expected cloudInit in body, got %s", receivedBody)
	}
	var data map[string]interface{}
	json.Unmarshal(result, &data)
	if data["accepted"] != true {
		t.Errorf("expected accepted=true, got %v", data["accepted"])
	}
}

func TestUpdateCloudInitConfirmation(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			w.Write([]byte(`{"error":{"code":"confirmation_required","message":"confirm"},"confirmation":{"confirmation_id":"cnf_test123"}}`))
			return
		}
		if r.Header.Get("X-User-Api-Confirm") != "cnf_test123" {
			t.Errorf("expected X-User-Api-Confirm header on retry, got %s", r.Header.Get("X-User-Api-Confirm"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{"status":"updated"}}`))
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, err := client.UpdateCloudInit(t.Context(), "1077", "#cloud-config\npackage_update: true\n")
	if err != nil {
		t.Fatalf("UpdateCloudInit failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (initial + confirmation), got %d", callCount)
	}
}

func TestDeleteCloudInitConfirmation(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			w.Write([]byte(`{"error":{"code":"confirmation_required","message":"confirm"},"confirmation":{"confirmation_id":"cnf_del123"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{"status":"deleted"}}`))
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, err := client.DeleteCloudInit(t.Context(), "1077")
	if err != nil {
		t.Fatalf("DeleteCloudInit failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("expected Idempotency-Key header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[{"id":"a","status":200,"body":{"ok":true}},{"id":"b","status":200,"body":{"ok":true}}]}`))
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	reqs := []BatchSubRequest{
		{ID: "a", Method: "GET", Path: "/account"},
		{ID: "b", Method: "GET", Path: "/vms"},
	}
	results, err := client.Batch(t.Context(), reqs)
	if err != nil {
		t.Fatalf("Batch failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "a" || results[1].ID != "b" {
		t.Errorf("expected order preserved: a, b; got %s, %s", results[0].ID, results[1].ID)
	}
}

func TestBatchOverLimit(t *testing.T) {
	client := NewSHCClient("test-key", "http://localhost")
	reqs := make([]BatchSubRequest, 26)
	_, err := client.Batch(t.Context(), reqs)
	if err == nil {
		t.Fatal("expected error for >25 requests")
	}
}

func TestConfirmationStablePath(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			w.Write([]byte(`{"error":{"code":"confirmation_required"},"confirmation":{"confirmation_id":"cnf_stable_456"}}`))
			return
		}
		if r.Header.Get("X-User-Api-Confirm") != "cnf_stable_456" {
			t.Errorf("expected confirm header cnf_stable_456, got %s", r.Header.Get("X-User-Api-Confirm"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, err := client.DeleteCloudInit(t.Context(), "1077")
	if err != nil {
		t.Fatalf("stable-path confirmation failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestConfirmationLegacyFallback(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			w.Write([]byte(`{"error":{"code":"confirmation_required"},"confirmation":{"structuredContent":{"confirmation_id":"cnf_legacy_789"}}}`))
			return
		}
		if r.Header.Get("X-User-Api-Confirm") != "cnf_legacy_789" {
			t.Errorf("expected confirm header cnf_legacy_789, got %s", r.Header.Get("X-User-Api-Confirm"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, err := client.DeleteCloudInit(t.Context(), "1077")
	if err != nil {
		t.Fatalf("legacy-path fallback failed: %v", err)
	}
}
