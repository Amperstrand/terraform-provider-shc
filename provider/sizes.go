package provider

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// The size map is generated, not hand-written: it is parsed at init from the
// embedded catalog.json artifact (schema shc-catalog/1), produced by
// shc-toolkit's scripts/generate-catalog-json.py from its static catalog
// model. Cross-repo size parity is therefore by construction — update the
// artifact (copy from shc-toolkit) instead of editing entries here.
//
//go:embed catalog.json
var catalogJSON []byte

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

type catalogArtifactDoc struct {
	Schema         string `json:"schema"`
	APIVersion     string `json:"api_version"`
	ToolkitVersion string `json:"toolkit_version"`
	Lines          map[string]struct {
		Label          string `json:"label"`
		OrderFormID    int64  `json:"order_form_id"`
		ModuleGroupID  int64  `json:"module_group_id"`
		PackageGroupID int64  `json:"package_group_id"`
	} `json:"lines"`
	Packages []struct {
		PackageID      int64            `json:"package_id"`
		Name           string           `json:"name"`
		Line           string           `json:"line"`
		Spec           string           `json:"spec"`
		CPU            int64            `json:"cpu"`
		RamMB          int64            `json:"ram_mb"`
		DiskGB         int64            `json:"disk_gb"`
		DailyPrice     string           `json:"daily_price"`
		PricingIDDaily int64            `json:"pricing_id_daily"`
		OptionIDs      map[string]int64 `json:"option_ids"`
		Templates      []string         `json:"templates"`
	} `json:"packages"`
}

var catalogArtifact catalogArtifactDoc

// sizeMap is the parsed artifact; a corrupt embedded artifact is a
// programmer error (pinned by sizes_catalog_test.go), so init fails loudly.
var sizeMap = mustParseSizeMap()

func mustParseSizeMap() map[string]sizeEntry {
	if err := json.Unmarshal(catalogJSON, &catalogArtifact); err != nil {
		panic(fmt.Sprintf("terraform-provider-shc: embedded catalog.json does not parse: %v", err))
	}
	lineOrderFormIDs = make(map[string]int64, len(catalogArtifact.Lines))
	lineModuleGroupIDs = make(map[string]int64, len(catalogArtifact.Lines))
	linePackageGroupIDs = make(map[string]int64, len(catalogArtifact.Lines))
	for line, info := range catalogArtifact.Lines {
		lineOrderFormIDs[line] = info.OrderFormID
		lineModuleGroupIDs[line] = info.ModuleGroupID
		linePackageGroupIDs[line] = info.PackageGroupID
	}

	m := make(map[string]sizeEntry, len(catalogArtifact.Packages))
	for _, p := range catalogArtifact.Packages {
		var price float64
		if _, err := fmt.Sscanf(p.DailyPrice, "%g", &price); err != nil {
			panic(fmt.Sprintf("terraform-provider-shc: catalog.json package %d has unparseable daily_price %q", p.PackageID, p.DailyPrice))
		}
		m[p.Spec] = sizeEntry{
			PackageID:  p.PackageID,
			PricingID:  p.PricingIDDaily,
			CPU:        p.CPU,
			RamMB:      p.RamMB,
			DiskGB:     p.DiskGB,
			Line:       p.Line,
			Name:       p.Name,
			DailyPrice: price,
		}
	}
	return m
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

// knownTemplates is generated from catalog_model — do not edit by hand.
var knownTemplates = []string{
	"almalinux10-cloud",
	"almalinux9-cloud",
	"alpine323-cloud",
	"arch-cloud",
	"cs10-cloud",
	"debian12-cloud",
	"debian13-cloud",
	"devuan5-cloud",
	"fedora42-cloud",
	"fedora43-cloud",
	"firecracker-cloud",
	"freebsd14-cloud",
	"gentoo-cloud",
	"kali-cloud",
	"netbsd10-cloud",
	"nixos-cloud",
	"ol10-cloud",
	"ol9-cloud",
	"openbsd79-cloud",
	"opensuse-leap156-cloud",
	"openwrt-cloud",
	"pve-ve-cloud",
	"rocky10-cloud",
	"rocky9-cloud",
	"ubuntu2204-cloud",
	"ubuntu2404-cloud",
	"ubuntu2604-cloud",
	"win11-pro-byol",
	"win2022-byol",
	"win2022-core-byol",
	"win2025-byol",
	"win2025-core-byol",
}

// Storefront triples per line, derived from the embedded catalog artifact.
// (SHC validates order_form_id together with module_group_id/package_group_id
// against the plan's storefront path, and the order-time ssh_key only
// survives the FULL triple — a lone form id 400s (form 11) or silently drops
// the key (forms 1/7). Values captured from live shc order --dry-run
// normalized_request, 2026-08-21.)
var lineOrderFormIDs map[string]int64

// Storefront triples per line (SHC validates order_form_id together with
// module_group_id/package_group_id against the plan's storefront path, and
// the order-time ssh_key only survives the FULL triple — a lone form id
// 400s (form 11) or silently drops the key (forms 1/7). Values captured
// from live shc order --dry-run normalized_request, 2026-08-21).
var lineModuleGroupIDs map[string]int64

var linePackageGroupIDs map[string]int64

func orderFormIDForPackage(packageID int64) (int64, bool) {
	for _, s := range sizeMap {
		if s.PackageID == packageID {
			if formID, ok := lineOrderFormIDs[s.Line]; ok {
				return formID, true
			}
		}
	}
	return 0, false
}

func dailyPriceForPackage(packageID int64) (float64, bool) {
	for _, s := range sizeMap {
		if s.PackageID == packageID {
			return s.DailyPrice, true
		}
	}
	return 0, false
}
