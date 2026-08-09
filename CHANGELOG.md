# Changelog

All notable changes to terraform-provider-shc are documented here.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] — 2026-08-09

### Added
- `id` computed attribute on VM, Snapshot, Firewall resources (enables import)
- UseStateForUnknown plan modifiers on VM computed attributes (ip, service_id, os_user)
- Structured error diagnostics (`provider/errors.go`) — parses SHC API JSON errors into summary + field-level detail
- go-retryablehttp transport (replaces custom retry loop, industry standard)
- tflog structured logging on ALL resources (snapshot, firewall, rdns, backup + VM)
- Data source acceptance tests: catalog, templates, VM data source
- Acceptance tests: PowerState (stop/start), Template (ubuntu2404), EdgeCases (negative CPU/RAM/disk)
- External firewall port verification: TCP-connect to verify rules work (open AND blocked)
- VM Disappears test: external deletion detection via API
- Benchmark results with sysbench CPU + fio 4K random/sequential disk
- Benchmark methodology disclaimer with reproduction steps
- Acceptance test sweepers for VM cleanup
- CheckDestroy functions for VM, Snapshot, Firewall
- `.gitignore` for security/fuzz test patterns

### Fixed
- Snapshot delete: poll GetSnapshots for real ID after async creation (was sending empty ID)
- Firewall position drift: UseStateForUnknown replaces RequiresReplace
- Firewall dest_port JSON tag (was "port", API returns "dest_port")
- Firewall Read: preserve state when API returns empty fields
- UpgradeVM: retry entire confirmation+upgrade flow on service_not_active
- UpgradeVM: auto_cancel=false in test prevents SHC API from cancelling VM during retry
- VM Import: set service_id from import ID (was only setting custom id attr)

### Changed
- go-retryablehttp replaces manual retry loop in doRequest
- All resource Read methods normalize API action/protocol to lowercase

## [0.2.0] — 2026-08-07

### Added
- HTTP retry on 429/503 with exponential backoff + ±20% jitter (matches Python client)
- Input validators: hostname (RFC 1123), size (regex `{line}-{cpu}c-{ram}gb`), positive-int on disk_gb/ram_mb/cpu
- Dynamic `order_form_id` resolution from catalog (was hardcoded to 11 — broke all non-Dev-VPS plans)
- `Idempotency-Key` header on order submissions (matches Python client, prevents duplicate orders)
- `PayInvoice` method for SHC invoice checkout flow
- GoReleaser config (`.goreleaser.yml`) for cross-platform binary releases
- Cross-compiled binaries: darwin/linux × amd64/arm64
- Production-readiness audit report (`docs/audit/extensive-provider-audit.md`)
- Acceptance test config fixed: NVMe Starter + debian12-cloud (was Dev VPS + no template)
- Pulumi TF Bridge SDK generation verified (`pulumi package add terraform-provider`)

### Changed
- `waitForProvisioning`: combined readiness check (`provisioning_state == "ready"` OR `status == "active" && ip != ""`) — fixes AGENTS.md Lesson #1
- `package_id` and `pricing_id` now `Computed: true` (provider resolves from `size` when user omits them)
- `term` attribute: removed `Computed` flag (provider never populated it; caused Pulumi bridge error)

### Removed
- NoDNS integration completely removed from the provider (4 schema fields, `publishNoDNS` function, call site in Create, state references, `bytes`/`os/exec` imports). NoDNS belongs at a provisioning layer above Terraform, not in the provider itself.

### Fixed
- Order submission now works for ALL plan types (NVMe/HDD/SSD/Dev) via dynamic order_form_id
- VM provisioning detection: SHC VMs stay in `provisioning_state: "provisioning"` forever — now correctly detects readiness via `service_status == "active" && ips != []`
- Confirmation flow: Idempotency-Key persists across initial request and confirmation re-send (was regenerated, causing API to reject confirmation)
- Acceptance test assertions: `status == "active"` instead of `provisioning_state == "ready"` (which never happens)

## [0.1.0] — 2026-07-02

Initial release with:
- VM lifecycle (create/read/update/delete) with spec-encoding size names
- Snapshot, Backup, Firewall rule, rDNS resources
- Catalog and Templates data sources
- Cost audit (CostTracker) with balance-diff tracking
- Config options (disk_gb, ram_mb, cpu, template) via ResolveAddons
- Size-map drift detection CI
- 57 unit tests
