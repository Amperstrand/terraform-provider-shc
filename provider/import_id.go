package provider

import (
	"fmt"
	"strings"
)

// parseImportID splits a compound Terraform import ID ("first:second") into
// its two non-empty parts. format names the expected shape for the error
// message (e.g. "service_id:snapshot_id").
func parseImportID(id, format string) (string, string, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("Expected import ID in the format %s.", format)
	}
	return parts[0], parts[1], nil
}
