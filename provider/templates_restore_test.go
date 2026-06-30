package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTemplates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/templates" {
			t.Errorf("expected path /vm/templates, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"name": "debian13-cloud", "family": "debian", "arch": "x86_64", "status": "active"},
					{"name": "ubuntu2404-cloud", "family": "ubuntu", "arch": "x86_64", "status": "active"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	templates, err := client.GetTemplates(context.Background())
	if err != nil {
		t.Fatalf("GetTemplates failed: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
	if templates[0].Name != "debian13-cloud" {
		t.Errorf("expected name debian13-cloud, got %s", templates[0].Name)
	}
	if templates[0].Family != "debian" {
		t.Errorf("expected family debian, got %s", templates[0].Family)
	}
	if templates[0].Arch != "x86_64" {
		t.Errorf("expected arch x86_64, got %s", templates[0].Arch)
	}
	if templates[0].Status != "active" {
		t.Errorf("expected status active, got %s", templates[0].Status)
	}
	if templates[1].Name != "ubuntu2404-cloud" {
		t.Errorf("expected name ubuntu2404-cloud, got %s", templates[1].Name)
	}
}

func TestGetTemplates_ArrayFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"name": "alpine323-cloud", "family": "alpine", "arch": "aarch64", "status": "active"},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	templates, err := client.GetTemplates(context.Background())
	if err != nil {
		t.Fatalf("GetTemplates failed: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}
	if templates[0].Name != "alpine323-cloud" {
		t.Errorf("expected name alpine323-cloud, got %s", templates[0].Name)
	}
}

func TestGetCatalog_WithPricing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"package_id": 23,
						"name":       "NVMe Starter",
						"cpu":        1,
						"memory_mb":  2048,
						"disk_gb":    25,
						"pricing": []map[string]interface{}{
							{"period": "day", "price": "0.50", "pricing_id": 55},
							{"period": "week", "price": "3.00", "pricing_id": 57},
							{"period": "month", "price": "10.00", "pricing_id": 56},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	packages, err := client.GetCatalog(context.Background())
	if err != nil {
		t.Fatalf("GetCatalog failed: %v", err)
	}
	if len(packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(packages))
	}
	pkg := packages[0]
	if len(pkg.Pricing) != 3 {
		t.Fatalf("expected 3 pricing entries, got %d", len(pkg.Pricing))
	}

	daily := priceForPeriod(pkg.Pricing, "day")
	if daily != "0.50" {
		t.Errorf("expected daily price 0.50, got %s", daily)
	}
	weekly := priceForPeriod(pkg.Pricing, "week")
	if weekly != "3.00" {
		t.Errorf("expected weekly price 3.00, got %s", weekly)
	}
	monthly := priceForPeriod(pkg.Pricing, "month")
	if monthly != "10.00" {
		t.Errorf("expected monthly price 10.00, got %s", monthly)
	}

	missing := priceForPeriod(pkg.Pricing, "year")
	if missing != "" {
		t.Errorf("expected empty price for unknown period, got %s", missing)
	}
}

func TestRestoreSnapshot(t *testing.T) {
	var receivedSnapshotID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/snapshots/restore" {
			t.Errorf("expected path /vm/123/snapshots/restore, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		receivedSnapshotID = body["snapshot_id"]

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.RestoreSnapshot(context.Background(), "123", "snap-456")
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}
	if receivedSnapshotID != "snap-456" {
		t.Errorf("expected snapshot_id snap-456, got %s", receivedSnapshotID)
	}
}

func TestRestoreSnapshot_ConfirmationFlow(t *testing.T) {
	callCount := 0
	var receivedConfirmID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"confirmation": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"confirmation_id": "conf-restore",
					},
				},
			})
			return
		}
		receivedConfirmID = r.Header.Get("X-User-Api-Confirm")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.RestoreSnapshot(context.Background(), "123", "snap-456")
	if err != nil {
		t.Fatalf("RestoreSnapshot with confirmation failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
	if receivedConfirmID != "conf-restore" {
		t.Errorf("expected confirm ID 'conf-restore', got %q", receivedConfirmID)
	}
}

func TestRestoreBackup(t *testing.T) {
	var receivedBackupID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/backups/restore" {
			t.Errorf("expected path /vm/123/backups/restore, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		receivedBackupID = body["backup_id"]

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.RestoreBackup(context.Background(), "123", "backup-789")
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	if receivedBackupID != "backup-789" {
		t.Errorf("expected backup_id backup-789, got %s", receivedBackupID)
	}
}

func TestRestoreBackup_ConfirmationFlow(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"confirmation": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"confirmation_id": "conf-backup-restore",
					},
				},
			})
			return
		}
		if r.Header.Get("X-User-Api-Confirm") != "conf-backup-restore" {
			t.Errorf("expected confirm header 'conf-backup-restore', got %q", r.Header.Get("X-User-Api-Confirm"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.RestoreBackup(context.Background(), "123", "backup-789")
	if err != nil {
		t.Fatalf("RestoreBackup with confirmation failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}
