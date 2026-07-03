package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveAddons_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ordering/catalog" {
			t.Errorf("expected /ordering/catalog, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"package_id": 26,
						"name":       "NVMe VPS - Standard",
						"cpu":        2,
						"memory_mb":  8192,
						"disk_gb":    16,
						"available_config_options": []map[string]interface{}{
							{
								"pricing_id": 56,
								"term":       1,
								"period":     "day",
								"options": []map[string]interface{}{
									{
										"option_id": 110,
										"name":      "ram",
										"label":     "Total RAM",
										"values": []map[string]interface{}{
											{"value": "8192", "name": "8 GB (Base)", "default": true},
											{"value": "16384", "name": "16 GB", "default": false},
										},
									},
									{
										"option_id": 112,
										"name":      "disk",
										"label":     "Disk Space",
										"values": []map[string]interface{}{
											{"value": "16", "name": "16 GB (Base)", "default": true},
											{"value": "50", "name": "50 GB", "default": false},
											{"value": "100", "name": "100 GB", "default": false},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	opts, err := client.ResolveAddons(
		context.Background(), 26,
		types.Int64Value(100),
		types.Int64Null(),
		types.Int64Null(),
		types.StringNull(),
	)
	if err != nil {
		t.Fatalf("ResolveAddons failed: %v", err)
	}
	if opts["112"] != "100" {
		t.Errorf("expected disk option 112=100, got %v", opts)
	}
	if _, hasRam := opts["110"]; hasRam {
		t.Errorf("should not include ram when ram_mb is null, got %v", opts)
	}
}

func TestResolveAddons_MultipleSpecs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"package_id": 26,
						"available_config_options": []map[string]interface{}{
							{
								"options": []map[string]interface{}{
									{
										"option_id": 110,
										"name":      "ram",
										"values": []map[string]interface{}{
											{"value": "8192"},
											{"value": "16384"},
										},
									},
									{
										"option_id": 111,
										"name":      "cpu",
										"values": []map[string]interface{}{
											{"value": "2"},
											{"value": "4"},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	opts, err := client.ResolveAddons(
		context.Background(), 26,
		types.Int64Null(),
		types.Int64Value(16384),
		types.Int64Value(4),
		types.StringNull(),
	)
	if err != nil {
		t.Fatalf("ResolveAddons failed: %v", err)
	}
	if opts["110"] != "16384" {
		t.Errorf("expected ram 110=16384, got %v", opts)
	}
	if opts["111"] != "4" {
		t.Errorf("expected cpu 111=4, got %v", opts)
	}
}

func TestResolveAddons_InvalidValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"package_id": 26,
						"available_config_options": []map[string]interface{}{
							{
								"options": []map[string]interface{}{
									{
										"option_id": 112,
										"name":      "disk",
										"values": []map[string]interface{}{
											{"value": "16"},
											{"value": "50"},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, err := client.ResolveAddons(
		context.Background(), 26,
		types.Int64Value(999),
		types.Int64Null(),
		types.Int64Null(),
		types.StringNull(),
	)
	if err == nil {
		t.Fatal("expected error for invalid disk value 999")
	}
}

func TestResolveAddons_PackageNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"package_id": 999},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	_, err := client.ResolveAddons(
		context.Background(), 26,
		types.Int64Value(50),
		types.Int64Null(),
		types.Int64Null(),
		types.StringNull(),
	)
	if err == nil {
		t.Fatal("expected error for package not found")
	}
}

func TestResolveAddons_NoSpecs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"package_id": 26},
				},
			},
		})
	}))
	defer server.Close()

	client := NewSHCClient("test-key", server.URL)
	opts, err := client.ResolveAddons(
		context.Background(), 26,
		types.Int64Null(),
		types.Int64Null(),
		types.Int64Null(),
		types.StringNull(),
	)
	if err != nil {
		t.Fatalf("expected no error when no specs provided: %v", err)
	}
	if len(opts) != 0 {
		t.Errorf("expected empty opts, got %v", opts)
	}
}
