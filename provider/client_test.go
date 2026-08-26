package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmitOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header 'Bearer test-key', got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path == "/ordering/catalog" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"items": []map[string]interface{}{
						{"package_id": "81", "order_form_id": "11"},
					},
				},
			})
			return
		}
		if r.URL.Path != "/ordering/submit" {
			t.Errorf("expected path /ordering/submit, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"service_ids": []string{"123"},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	order, err := client.SubmitOrder(context.Background(), "test-vm", 81, 245, nil, "")
	if err != nil {
		t.Fatalf("SubmitOrder failed: %v", err)
	}
	if order.ResolveServiceID() != "123" {
		t.Errorf("expected service_id 123, got %s", order.ResolveServiceID())
	}
}

func TestSubmitOrder_ConfirmationFlow(t *testing.T) {
	callCount := 0
	var sawSSHyKey bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req struct {
			SSHKey string `json:"ssh_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.SSHKey != "" {
			sawSSHyKey = true
		}
		// POST /ordering/submit without confirm → 409
		if r.URL.Path == "/ordering/submit" && r.Header.Get("X-User-Api-Confirm") == "" {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"confirmation": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"confirmation_id": "conf-abc",
					},
				},
			})
			return
		}
		if r.Header.Get("X-User-Api-Confirm") != "conf-abc" {
			t.Errorf("expected confirm header 'conf-abc', got %q", r.Header.Get("X-User-Api-Confirm"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"service_ids": []string{"456"},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	order, err := client.SubmitOrder(context.Background(), "test-vm", 81, 245, nil, "ssh-ed25519 AAAAtest")
	if err != nil {
		t.Fatalf("SubmitOrder with confirmation failed: %v", err)
	}
	if order.ResolveServiceID() != "456" {
		t.Errorf("expected service_id 456, got %s", order.ResolveServiceID())
	}
	// 2 calls: initial POST (409) + confirmation POST (no catalog fetch — static map)
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
	if !sawSSHyKey {
		t.Error("ssh_key must ride the order request body (order-time injection)")
	}
}

func TestGetVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/vm/123" {
			t.Errorf("expected path /vm/123, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"service_id":         "123",
				"hostname":           "test-vm",
				"os_user":            "debian",
				"service_status":     "active",
				"provisioning_state": "ready",
				"ips": []map[string]string{
					{"ip": "1.2.3.4"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	vm, err := client.GetVM(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetVM failed: %v", err)
	}
	if vm.GetIP() != "1.2.3.4" {
		t.Errorf("expected ip 1.2.3.4, got %s", vm.GetIP())
	}
	if vm.Hostname != "test-vm" {
		t.Errorf("expected hostname test-vm, got %s", vm.Hostname)
	}
	if vm.OSUser != "debian" {
		t.Errorf("expected os_user debian, got %s", vm.OSUser)
	}
	if vm.Status != "active" {
		t.Errorf("expected status active, got %s", vm.Status)
	}
	if vm.ProvisioningState != "ready" {
		t.Errorf("expected provisioning_state ready, got %s", vm.ProvisioningState)
	}
}

func TestGetVM_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, err := client.GetVM(context.Background(), "999")
	if err != ErrVMNotFound {
		t.Errorf("expected ErrVMNotFound, got %v", err)
	}
}

func TestCancelVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/cancel" {
			t.Errorf("expected path /vm/123/cancel, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.CancelVM(context.Background(), "123", true)
	if err != nil {
		t.Fatalf("CancelVM failed: %v", err)
	}
}

func TestCancelVM_ConfirmationFlow(t *testing.T) {
	callCount := 0
	var receivedConfirmID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			if r.Header.Get("X-User-Api-Confirm") != "" {
				t.Errorf("first call should not have confirm header")
			}
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"confirmation": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"confirmation_id": "conf-cancel-xyz",
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
	err := client.CancelVM(context.Background(), "123", true)
	if err != nil {
		t.Fatalf("CancelVM with confirmation failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
	if receivedConfirmID != "conf-cancel-xyz" {
		t.Errorf("expected confirm ID 'conf-cancel-xyz', got %q", receivedConfirmID)
	}
}

func TestGetSnapshots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/snapshots" {
			t.Errorf("expected path /vm/123/snapshots, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"id": "snap-1", "name": "first", "status": "done", "date": "2025-01-01"},
					{"id": "snap-2", "name": "second", "status": "pending", "date": "2025-01-02"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	snaps, err := client.GetSnapshots(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetSnapshots failed: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}
	if snaps[0].ID.String() != "snap-1" {
		t.Errorf("expected first snapshot ID 'snap-1', got %s", snaps[0].ID.String())
	}
	if snaps[0].Name != "first" {
		t.Errorf("expected first snapshot name 'first', got %s", snaps[0].Name)
	}
	if snaps[1].Status != "pending" {
		t.Errorf("expected second snapshot status 'pending', got %s", snaps[1].Status)
	}
}

func TestGetSnapshots_ArrayFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "s1", "name": "a", "status": "done"},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	snaps, err := client.GetSnapshots(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetSnapshots failed: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].ID.String() != "s1" {
		t.Errorf("expected snapshot ID 's1', got %s", snaps[0].ID.String())
	}
}

func TestGetBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/billing/balance" {
			t.Errorf("expected path /billing/balance, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"balance":  "10.50",
				"credit":   []map[string]string{{"currency": "USD", "amount": "5.00"}},
				"currency": "USD",
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	bal, err := client.GetBalance(context.Background())
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if bal.Balance.String() != "10.50" {
		t.Errorf("expected balance 10.50, got %s", bal.Balance.String())
	}
	if len(bal.Credit) == 0 || bal.Credit[0].Amount.String() != "5.00" {
		t.Errorf("expected credit 5.00, got %+v", bal.Credit)
	}
	if bal.Currency != "USD" {
		t.Errorf("expected currency USD, got %s", bal.Currency)
	}
}

// Regression: the LIVE /billing/balance response (captured 2026-08-21,
// eddy-lab deploy) returns credit as an ARRAY with balances alongside.
// A scalar Credit field failed json.Unmarshal on the whole response and
// blocked every order with "credit check failed (cannot verify balance)".
func TestGetBalance_LiveCreditArrayShape(t *testing.T) {
	liveBody := `{"data":{"default_currency":"USD","selected_currency":"USD","payment_credit_enabled":true,"balances":[{"currency":"USD","available_credit":"72.31","open_invoices_total":"0.00","open_invoices_paid":"0.00","balance_due":"0.00"}],"credit":[{"currency":"USD","amount":"72.31"}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(liveBody))
	}))
	defer server.Close()

	client := NewSHCClient("test", server.URL)
	bal, err := client.GetBalance(context.Background())
	if err != nil {
		t.Fatalf("live-shape balance must parse, got: %v", err)
	}
	if len(bal.Credit) != 1 || bal.Credit[0].Amount != "72.31" {
		t.Errorf("credit: expected [{USD 72.31}], got %+v", bal.Credit)
	}
	if err := client.CheckCredit(context.Background(), 50.0); err != nil {
		t.Errorf("72.31 available must pass a $50 check, got: %v", err)
	}
	if err := client.CheckCredit(context.Background(), 100.0); err == nil {
		t.Error("72.31 available must fail a $100 check")
	}
}

func TestCheckCredit_Insufficient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"balances": []map[string]string{
					{"currency": "USD", "available_credit": "0.10"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test", server.URL)
	err := client.CheckCredit(context.Background(), 0.50)
	if err == nil {
		t.Fatal("expected error for insufficient credit, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient credit") {
		t.Errorf("expected 'insufficient credit' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "0.10") {
		t.Errorf("expected available amount in error, got: %v", err)
	}
}

func TestCheckCredit_Sufficient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"balances": []map[string]string{
					{"currency": "USD", "available_credit": "5.00"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test", server.URL)
	err := client.CheckCredit(context.Background(), 0.50)
	if err != nil {
		t.Fatalf("expected nil for sufficient credit, got: %v", err)
	}
}

func TestCheckCredit_FailsClosedOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewSHCClient("test", server.URL)
	err := client.CheckCredit(context.Background(), 0.50)
	if err == nil {
		t.Fatal("expected error when balance endpoint fails (fail closed)")
	}
}

func TestGetCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ordering/catalog" {
			t.Errorf("expected path /ordering/catalog, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"package_id": 81, "name": "Standard", "cpu": 2, "memory_mb": 4096, "disk_gb": 50},
					{"package_id": 82, "name": "Professional", "cpu": 4, "memory_mb": 8192, "disk_gb": 100},
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
	if len(packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(packages))
	}
	if packages[0].PackageID != 81 {
		t.Errorf("expected package_id 81, got %d", packages[0].PackageID)
	}
	if packages[0].Name != "Standard" {
		t.Errorf("expected name Standard, got %s", packages[0].Name)
	}
	if packages[0].CPU != 2 {
		t.Errorf("expected cpu 2, got %d", packages[0].CPU)
	}
	if packages[0].MemoryMB != 4096 {
		t.Errorf("expected memory_mb 4096, got %d", packages[0].MemoryMB)
	}
	if packages[0].DiskGB != 50 {
		t.Errorf("expected disk_gb 50, got %d", packages[0].DiskGB)
	}
	if packages[1].Name != "Professional" {
		t.Errorf("expected name Professional, got %s", packages[1].Name)
	}
}

func TestGetCatalog_ArrayFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"package_id": 23, "name": "NVMe Starter", "cpu": 1, "memory_mb": 2048, "disk_gb": 25},
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
	if packages[0].PackageID != 23 {
		t.Errorf("expected package_id 23, got %d", packages[0].PackageID)
	}
}

func TestRetryOnLock_RetriesOnLocked(t *testing.T) {
	callCount := 0
	err := retryOnLock(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return fmt.Errorf("VM is locked (backup)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retries, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetryOnLock_DoesNotRetryNonLockedError(t *testing.T) {
	callCount := 0
	err := retryOnLock(context.Background(), func() error {
		callCount++
		return fmt.Errorf("some other error")
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "some other error") {
		t.Errorf("expected original error to be preserved, got: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

func TestRetryOnLock_ExhaustsRetries(t *testing.T) {
	callCount := 0
	err := retryOnLock(context.Background(), func() error {
		callCount++
		return fmt.Errorf("VM is locked (backup)")
	})
	if err == nil {
		t.Fatalf("expected error after retries exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "VM is locked by a running job") {
		t.Errorf("expected locked message, got: %v", err)
	}
	if callCount != lockMaxRetries+1 {
		t.Errorf("expected %d calls (1 initial + %d retries), got %d", lockMaxRetries+1, lockMaxRetries, callCount)
	}
}

func TestRetryOnLockValue_RetriesOnLocked(t *testing.T) {
	callCount := 0
	result, err := retryOnLockValue(context.Background(), func() (string, error) {
		callCount++
		if callCount < 2 {
			return "", fmt.Errorf("VM is locked (snapshot)")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retry, got: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected result 'ok', got %q", result)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestCancelVM_RetriesOnLocked(t *testing.T) {
	callCount := 0
	err := retryOnLock(context.Background(), func() error {
		callCount++
		if callCount < 2 {
			return errors.New("VM is locked (backup)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil after retry, got: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestIsVMLockedErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"locked backup", fmt.Errorf("VM is locked (backup)"), true},
		{"locked snapshot", fmt.Errorf("vm is locked (snapshot)"), true},
		{"locked lowercase", fmt.Errorf("the vm is locked by a running job"), true},
		{"not locked", fmt.Errorf("some other error"), false},
		{"not found", fmt.Errorf("not found"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVMLockedErr(tt.err); got != tt.want {
				t.Errorf("isVMLockedErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestUpgradeVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/upgrade" {
			t.Errorf("expected path /vm/123/upgrade, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["pricing_ref"] != float64(249) {
			t.Errorf("expected pricing_ref 249, got %v", body["pricing_ref"])
		}
		if body["idempotency_key"] == nil || body["idempotency_key"] == "" {
			t.Errorf("expected non-empty idempotency_key")
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"applies": "queued",
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.UpgradeVM(context.Background(), "123", 249)
	if err != nil {
		t.Fatalf("UpgradeVM failed: %v", err)
	}
}

func TestUpgradeVM_ConfirmationFlow(t *testing.T) {
	callCount := 0
	var receivedConfirmID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			if r.Header.Get("X-User-Api-Confirm") != "" {
				t.Errorf("first call should not have confirm header")
			}
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"confirmation": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"confirmation_id": "conf-upgrade-xyz",
					},
				},
			})
			return
		}
		receivedConfirmID = r.Header.Get("X-User-Api-Confirm")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"applies": "queued",
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.UpgradeVM(context.Background(), "123", 249)
	if err != nil {
		t.Fatalf("UpgradeVM with confirmation failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
	if receivedConfirmID != "conf-upgrade-xyz" {
		t.Errorf("expected confirm ID 'conf-upgrade-xyz', got %q", receivedConfirmID)
	}
}

func TestUserAgent(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": {}}`))
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	if _, err := client.GetOrder(context.Background(), "1"); err != nil {
		t.Fatalf("default UA request failed: %v", err)
	}
	if gotUA != defaultUserAgent {
		t.Errorf("expected default UA %q, got %q", defaultUserAgent, gotUA)
	}

	client.SetUserAgent("terraform-provider-shc/9.9.9-test (SHC API v2.4.24)")
	if _, err := client.GetOrder(context.Background(), "1"); err != nil {
		t.Fatalf("overridden UA request failed: %v", err)
	}
	if gotUA != "terraform-provider-shc/9.9.9-test (SHC API v2.4.24)" {
		t.Errorf("expected overridden UA, got %q", gotUA)
	}
}
