# Changelog

All notable changes to terraform-provider-shc are documented here.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Acceptance tests (TF_ACC) for VM resource: basic create, size, template, import
- Schema versioning: `Version: 1` with no-op StateUpgrader (0→1) on VM resource
- `term` attribute on VM resource (v2.4.3 VM term management)
- 10 new Go client methods (VM term/addons + Orders)
- GNUmakefile with `build`, `test`, `testacc`, `fmt` targets

### Fixed
- Size-map drift detection: dict comprehension error + heredoc bash quoting
- Size-map drift: skip when no API key (false positive on 401)
- Action versions bumped to v7/v6

## [Unreleased]

### Added
- Go client methods for v2.4.6: StandbyVM, PreviewStandby, ResumeVM, ListEvents
- `term` attribute on VM resource (v2.4.3 VM term management)
- Schema versioning: `Version: 1` with no-op StateUpgrader
- 4 TF_ACC acceptance tests (basic, size, template, import)
- GNUmakefile with build, test, testacc, fmt targets
- Acceptance CI workflow (workflow_dispatch + weekly schedule)

### Fixed
- Size-map drift detection: dict comprehension error + heredoc bash quoting
- Size-map drift: skip when no API key (false positive on 401)
- Integration cleanup step: continue-on-error + env guard

## [0.1.0] — 2026-07-02

Initial release with:
- VM lifecycle (create/read/update/delete) with spec-encoding size names
- Snapshot, Backup, Firewall rule, rDNS resources
- Catalog and Templates data sources
- Cost audit (CostTracker) with balance-diff tracking
- Config options (disk_gb, ram_mb, cpu, template) via ResolveAddons
- Size-map drift detection CI
- 57 unit tests
