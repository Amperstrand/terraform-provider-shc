package provider

import "fmt"

type sizeEntry struct {
	PackageID  int64
	PricingID  int64
	CPU        int64
	RamMB      int64
	DiskGB     int64
	Line       string
	Name       string
	DailyPrice float64
}
var sizeMap = map[string]sizeEntry{
	"nvme-1c-4gb":   {23, 55, 1, 4096, 8, "nvme", "NVMe VPS - Starter", 0.26},
	"nvme-2c-8gb":   {26, 56, 2, 8192, 16, "nvme", "NVMe VPS - Standard", 0.49},
	"nvme-4c-16gb":  {29, 57, 4, 16384, 32, "nvme", "NVMe VPS - Professional", 0.96},
	"nvme-8c-32gb":  {32, 58, 8, 32768, 64, "nvme", "NVMe VPS - Business", 1.91},
	"nvme-16c-64gb": {35, 59, 16, 65536, 128, "nvme", "NVMe VPS - Enterprise", 3.79},
	"hdd-1c-4gb":    {36, 67, 1, 4096, 8, "hdd", "HDD VPS - Starter", 0.24},
	"hdd-2c-8gb":    {37, 71, 2, 8192, 16, "hdd", "HDD VPS - Standard", 0.46},
	"hdd-4c-16gb":   {38, 75, 4, 16384, 32, "hdd", "HDD VPS - Professional", 0.90},
	"hdd-8c-32gb":   {39, 79, 8, 32768, 64, "hdd", "HDD VPS - Business", 1.78},
	"hdd-16c-64gb":  {40, 83, 16, 65536, 128, "hdd", "HDD VPS - Enterprise", 3.53},
	"ssd-1c-4gb":    {56, 147, 1, 4096, 8, "ssd", "SSD VPS - Starter", 0.24},
	"ssd-2c-8gb":    {57, 151, 2, 8192, 16, "ssd", "SSD VPS - Standard", 0.46},
	"ssd-4c-16gb":   {58, 155, 4, 16384, 32, "ssd", "SSD VPS - Professional", 0.90},
	"ssd-8c-32gb":   {59, 159, 8, 32768, 64, "ssd", "SSD VPS - Business", 1.78},
	"ssd-16c-64gb":  {60, 163, 16, 65536, 128, "ssd", "SSD VPS - Enterprise", 3.54},
	"dev-1c-4gb":    {80, 241, 1, 4096, 8, "dev", "Dev VPS - Starter", 0.24},
	"dev-2c-8gb":    {81, 245, 2, 8192, 16, "dev", "Dev VPS - Standard", 0.46},
	"dev-4c-16gb":   {82, 249, 4, 16384, 32, "dev", "Dev VPS - Professional", 0.90},
	"dev-8c-32gb":   {83, 253, 8, 32768, 64, "dev", "Dev VPS - Business", 1.78},
	"dev-16c-64gb":  {84, 257, 16, 65536, 128, "dev", "Dev VPS - Enterprise", 3.54},
}

func resolveSize(size string) (int64, int64, error) {
	s, ok := sizeMap[size]
	if !ok {
		return 0, 0, fmt.Errorf("unknown size '%s'. Valid sizes: nvme-{1,2,4,8,16}c-{4,8,16,32,64}gb, ssd-*, hdd-*, dev-*", size)
	}
	return s.PackageID, s.PricingID, nil
}

func resolveSizeFull(size string) (pkgID, priceID, cpu, ramMB int64, diskGB int64, line string, dailyPrice float64, err error) {
	s, ok := sizeMap[size]
	if !ok {
		return 0, 0, 0, 0, 0, "", 0, fmt.Errorf("unknown size '%s'", size)
	}
	return s.PackageID, s.PricingID, s.CPU, s.RamMB, s.DiskGB, s.Line, s.DailyPrice, nil
}

func resolveSpecs(cpu, ramMB, diskGB int64, line string) (int64, int64, error) {
	lineRank := map[string]int{"nvme": 0, "ssd": 1, "hdd": 2, "dev": 3}
	var best *sizeEntry
	for _, s := range sizeMap {
		if line != "" && s.Line != line {
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
		if best == nil {
			best = &s
			continue
		}
		if s.DailyPrice < best.DailyPrice {
			best = &s
		} else if s.DailyPrice == best.DailyPrice && lineRank[s.Line] < lineRank[best.Line] {
			best = &s
		}
	}
	if best == nil {
		return 0, 0, fmt.Errorf("no plan matches: cpu>=%d, ram>=%dMB, disk>=%dGB, line=%s", cpu, ramMB, diskGB, line)
	}
	return best.PackageID, best.PricingID, nil
}

func dailyPriceForPackage(packageID int64) (float64, bool) {
	for _, s := range sizeMap {
		if s.PackageID == packageID {
			return s.DailyPrice, true
		}
	}
	return 0, false
}

var lineOrderFormIDs = map[string]int64{
	"nvme": 1,
	"ssd":  7,
	"hdd":  3,
	"dev":  11,
}

// Storefront triples per line (SHC validates order_form_id together with
// module_group_id/package_group_id against the plan's storefront path, and
// the order-time ssh_key only survives the FULL triple — a lone form id
// 400s (form 11) or silently drops the key (forms 1/7). Values captured
// from live shc order --dry-run normalized_request, 2026-08-21).
var lineModuleGroupIDs = map[string]int64{
	"nvme": 4,
	"ssd":  7,
	"hdd":  8,
	"dev":  7,
}

var linePackageGroupIDs = map[string]int64{
	"nvme": 3,
	"ssd":  10,
	"hdd":  5,
	"dev":  14,
}


func minDailyPrice() float64 {
	var min float64
	for _, s := range sizeMap {
		if min == 0 || s.DailyPrice < min {
			min = s.DailyPrice
		}
	}
	return min
}
