package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateFirewallRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header 'Bearer test-key', got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/vm/123/firewall/rules" {
			t.Errorf("expected path /vm/123/firewall/rules, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]string
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		if reqBody["action"] != "accept" {
			t.Errorf("expected action accept, got %s", reqBody["action"])
		}
		if reqBody["protocol"] != "tcp" {
			t.Errorf("expected protocol tcp, got %s", reqBody["protocol"])
		}
		if reqBody["port"] != "22" {
			t.Errorf("expected port 22, got %s", reqBody["port"])
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"position":  1,
				"action":    "accept",
				"protocol":  "tcp",
				"port":      "22",
				"source":    "0.0.0.0/0",
				"direction": "in",
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	reqBody := map[string]string{
		"action":    "accept",
		"protocol":  "tcp",
		"port":      "22",
		"source":    "0.0.0.0/0",
		"direction": "in",
	}
	body, _ := json.Marshal(reqBody)
	rule, err := client.CreateFirewallRule(context.Background(), "123", body)
	if err != nil {
		t.Fatalf("CreateFirewallRule failed: %v", err)
	}
	if rule.Position.Int64() != 1 {
		t.Errorf("expected position 1, got %d", rule.Position.Int64())
	}
	if rule.Action != "accept" {
		t.Errorf("expected action accept, got %s", rule.Action)
	}
}

func TestGetFirewall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/firewall" {
			t.Errorf("expected path /vm/123/firewall, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"rules": []map[string]interface{}{
					{"position": 1, "action": "accept", "protocol": "tcp", "port": "22", "source": "0.0.0.0/0", "direction": "in", "name": "ssh"},
					{"position": 2, "action": "drop", "protocol": "tcp", "port": "80", "source": "10.0.0.0/8", "direction": "in"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	fw, err := client.GetFirewall(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetFirewall failed: %v", err)
	}
	if len(fw.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(fw.Rules))
	}
	if fw.Rules[0].Position.Int64() != 1 {
		t.Errorf("expected first rule position 1, got %d", fw.Rules[0].Position.Int64())
	}
	if fw.Rules[0].Action != "accept" {
		t.Errorf("expected first rule action accept, got %s", fw.Rules[0].Action)
	}
	if fw.Rules[0].Name != "ssh" {
		t.Errorf("expected first rule name ssh, got %s", fw.Rules[0].Name)
	}
	if fw.Rules[1].Action != "drop" {
		t.Errorf("expected second rule action drop, got %s", fw.Rules[1].Action)
	}
}

func TestGetFirewall_ArrayFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"position": 5, "action": "reject", "protocol": "udp", "port": "53", "source": "0.0.0.0/0", "direction": "in"},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	fw, err := client.GetFirewall(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetFirewall failed: %v", err)
	}
	if len(fw.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(fw.Rules))
	}
	if fw.Rules[0].Position.Int64() != 5 {
		t.Errorf("expected position 5, got %d", fw.Rules[0].Position.Int64())
	}
}

func TestDeleteFirewallRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/123/firewall/rules/1" {
			t.Errorf("expected path /vm/123/firewall/rules/1, got %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.DeleteFirewallRule(context.Background(), "123", 1)
	if err != nil {
		t.Fatalf("DeleteFirewallRule failed: %v", err)
	}
}

func TestDeleteFirewallRule_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	err := client.DeleteFirewallRule(context.Background(), "123", 99)
	if err != nil {
		t.Fatalf("DeleteFirewallRule should return nil on 404, got: %v", err)
	}
}
