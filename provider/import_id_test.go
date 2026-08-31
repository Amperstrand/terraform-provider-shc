package provider

import "testing"

func TestParseImportID_whenValid(t *testing.T) {
	first, second, err := parseImportID("123:456", "service_id:backup_id")
	if err != nil {
		t.Fatalf("expected valid ID to parse, got error: %v", err)
	}
	if first != "123" || second != "456" {
		t.Errorf("expected (123, 456), got (%s, %s)", first, second)
	}
}

func TestParseImportID_whenMissingParts(t *testing.T) {
	cases := []string{"", "123", "123:", ":456", "123:456:789"}
	for _, id := range cases {
		if _, _, err := parseImportID(id, "service_id:backup_id"); err == nil {
			t.Errorf("expected error for %q, got none", id)
		}
	}
}

func TestParseImportID_errorNamesFormat(t *testing.T) {
	_, _, err := parseImportID("nope", "service_id:snapshot_id")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "Expected import ID in the format service_id:snapshot_id."
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
