package provider

import "testing"

func TestResolveSize(t *testing.T) {
	pkgID, priceID, err := resolveSize("nvme-2c-8gb")
	if err != nil {
		t.Fatalf("expected no error for 'nvme-2c-8gb', got: %v", err)
	}
	if pkgID != 26 {
		t.Errorf("expected package_id 26 for 'nvme-2c-8gb', got %d", pkgID)
	}
	if priceID != 56 {
		t.Errorf("expected pricing_id 56 for 'nvme-2c-8gb', got %d", priceID)
	}
}

func TestResolveSize_HDD(t *testing.T) {
	pkgID, priceID, err := resolveSize("hdd-1c-4gb")
	if err != nil {
		t.Fatalf("expected no error for 'hdd-1c-4gb', got: %v", err)
	}
	if pkgID != 36 {
		t.Errorf("expected package_id 36 for 'hdd-1c-4gb', got %d", pkgID)
	}
	if priceID != 67 {
		t.Errorf("expected pricing_id 67 for 'hdd-1c-4gb', got %d", priceID)
	}
}

func TestResolveSize_DevLine(t *testing.T) {
	pkgID, priceID, err := resolveSize("dev-4c-16gb")
	if err != nil {
		t.Fatalf("expected no error for 'dev-4c-16gb', got: %v", err)
	}
	if pkgID != 82 {
		t.Errorf("expected package_id 82 for 'dev-4c-16gb', got %d", pkgID)
	}
	if priceID != 249 {
		t.Errorf("expected pricing_id 249 for 'dev-4c-16gb', got %d", priceID)
	}
}

func TestResolveSize_LegacyAliasRejected(t *testing.T) {
	_, _, err := resolveSize("standard")
	if err == nil {
		t.Fatal("expected error for legacy alias 'standard', got nil")
	}
}

func TestResolveSize_Invalid(t *testing.T) {
	_, _, err := resolveSize("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown size 'nonexistent', got nil")
	}
}

func TestResolveSpecs_AllLines(t *testing.T) {
	pkgID, _, err := resolveSpecs(4, 16384, 0, "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if pkgID != 58 {
		t.Errorf("expected package_id 58 (SSD Pro cheapest across all lines), got %d", pkgID)
	}
}

func TestResolveSpecs_NVMeOnly(t *testing.T) {
	pkgID, priceID, err := resolveSpecs(2, 8192, 0, "nvme")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if pkgID != 26 {
		t.Errorf("expected package_id 26 (NVMe Standard), got %d", pkgID)
	}
	if priceID != 56 {
		t.Errorf("expected pricing_id 56, got %d", priceID)
	}
}

func TestResolveSpecs_NoMatch(t *testing.T) {
	_, _, err := resolveSpecs(32, 131072, 256, "")
	if err == nil {
		t.Fatal("expected error when specs exceed all plans, got nil")
	}
}
