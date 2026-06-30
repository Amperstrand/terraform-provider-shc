package provider

import "fmt"

// sizeEntry describes a single named plan in the static size catalogue.
type sizeEntry struct {
	PackageID int64
	PricingID int64
	CPU       int64
	RamMB     int64
	DiskGB    int64
	Line      string
	Name      string
}

// sizeMap is a static lookup table mapping human-readable size names to the
// underlying SHC package/pricing IDs. This avoids an API round-trip for the
// common case where a practitioner just wants a known plan tier.
var sizeMap = map[string]sizeEntry{
	"starter":          {23, 55, 1, 4096, 8, "nvme", "NVMe VPS - Starter"},
	"standard":         {26, 56, 2, 8192, 16, "nvme", "NVMe VPS - Standard"},
	"professional":     {29, 57, 4, 16384, 32, "nvme", "NVMe VPS - Professional"},
	"business":         {32, 58, 8, 32768, 64, "nvme", "NVMe VPS - Business"},
	"enterprise":       {35, 59, 16, 65536, 128, "nvme", "NVMe VPS - Enterprise"},
	"dev-starter":      {80, 241, 1, 4096, 8, "dev", "Dev VPS - Starter"},
	"dev-standard":     {81, 245, 2, 8192, 16, "dev", "Dev VPS - Standard"},
	"dev-professional": {82, 249, 4, 16384, 32, "dev", "Dev VPS - Professional"},
	"dev-business":     {83, 253, 8, 32768, 64, "dev", "Dev VPS - Business"},
	"dev-enterprise":   {84, 257, 16, 65536, 128, "dev", "Dev VPS - Enterprise"},
}

// resolveSize translates a named size into its package and pricing IDs.
func resolveSize(size string) (int64, int64, error) {
	s, ok := sizeMap[size]
	if !ok {
		return 0, 0, fmt.Errorf("unknown size '%s'. Valid: starter, standard, professional, business, enterprise, dev-*", size)
	}
	return s.PackageID, s.PricingID, nil
}

// resolveSpecs finds the cheapest NVMe plan that meets or exceeds the given
// minimum CPU, RAM, and disk requirements. A zero value for any dimension means
// "no constraint" on that dimension. Returns an error when no plan matches.
func resolveSpecs(cpu, ramMB, diskGB int64) (int64, int64, error) {
	// Find cheapest NVMe plan meeting specs. Plans are ordered by pricing_id
	// ascending, so the first match (lowest pricing_id) is the cheapest tier.
	var bestPricing int64
	var bestPkg int64
	found := false

	for _, s := range sizeMap {
		if s.Line != "nvme" {
			continue
		}
		if cpu > 0 && s.CPU < cpu {
			continue
		}
		if ramMB > 0 && s.RamMB < ramMB {
			continue
		}
		if diskGB > 0 && s.DiskGB < diskGB {
			continue
		}
		if !found || s.PricingID < bestPricing {
			bestPricing = s.PricingID
			bestPkg = s.PackageID
			found = true
		}
	}
	if !found {
		return 0, 0, fmt.Errorf("no plan matches: cpu>=%d, ram>=%dMB, disk>=%dGB", cpu, ramMB, diskGB)
	}
	return bestPkg, bestPricing, nil
}
