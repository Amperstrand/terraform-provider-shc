package provider

import "testing"

func TestResolveSize(t *testing.T) {
	pkgID, priceID, err := resolveSize("standard")
	if err != nil {
		t.Fatalf("expected no error for 'standard', got: %v", err)
	}
	if pkgID != 26 {
		t.Errorf("expected package_id 26 for 'standard', got %d", pkgID)
	}
	if priceID != 56 {
		t.Errorf("expected pricing_id 56 for 'standard', got %d", priceID)
	}
}

func TestResolveSize_DevLine(t *testing.T) {
	pkgID, priceID, err := resolveSize("dev-professional")
	if err != nil {
		t.Fatalf("expected no error for 'dev-professional', got: %v", err)
	}
	if pkgID != 82 {
		t.Errorf("expected package_id 82 for 'dev-professional', got %d", pkgID)
	}
	if priceID != 249 {
		t.Errorf("expected pricing_id 249 for 'dev-professional', got %d", priceID)
	}
}

func TestResolveSize_Invalid(t *testing.T) {
	_, _, err := resolveSize("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown size 'nonexistent', got nil")
	}
}

func TestResolveSpecs(t *testing.T) {
	// cpu=2, ram=8192MB should match 'standard' (pkg 26, pricing 56), the
	// cheapest NVMe plan meeting those minimums.
	pkgID, priceID, err := resolveSpecs(2, 8192, 0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if pkgID != 26 {
		t.Errorf("expected package_id 26, got %d", pkgID)
	}
	if priceID != 56 {
		t.Errorf("expected pricing_id 56, got %d", priceID)
	}
}

func TestResolveSpecs_NoMatch(t *testing.T) {
	// Specs exceeding the largest NVMe plan (enterprise: 16 CPU, 64GB RAM, 128GB disk).
	_, _, err := resolveSpecs(32, 131072, 256)
	if err == nil {
		t.Fatal("expected error when specs exceed all plans, got nil")
	}
}
