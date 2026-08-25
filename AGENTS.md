# AGENTS.md — Terraform Provider SHC Maintenance Guide

## Architecture

Go provider for Terraform, bridged to Pulumi via "Any Terraform Provider".

```
terraform-provider-shc (Go)
├── provider/client.go           — SHCClient (HTTP, retry, order submission)
├── provider/sizes.go            — Static size map (generated from shc-toolkit/catalog_model.py)
├── provider/vm_client.go        — VM CRUD + addon resolution (live catalog)
├── provider/catalog_client.go   — Balance, credit check, EstimateDailyCost (static)
├── provider/vm_resource.go      — Terraform resource schema + lifecycle
└── provider/types.go            — Request/response types
```

## Regenerating sizes.go

```bash
# From shc-toolkit (sibling checkout):
python3 ../shc-toolkit/scripts/generate_sizes.py --format go --output provider/sizes.go
```

The script reads from `catalog_model.py` (no network call). Prices match the live API exactly (20/20) using Decimal arithmetic with `ROUND_HALF_UP`.

## When SHC ships an API update

1. Update `shc-toolkit/openapi.json` from the live spec.
2. Regenerate Python client: `bash scripts/generate_client.sh`
3. Validate catalog model: `SHC_API_KEY=... python3 scripts/validate_catalog_model.py`
4. Regenerate Go sizes: `python3 ../shc-toolkit/scripts/generate_sizes.py --format go --output provider/sizes.go`
5. Update Go client methods if new endpoints were added.
6. Run: `go test ./provider/ -count=1`
7. Run: `go vet ./provider/`
8. **Add a CHANGELOG entry.**

## CI workflows

**Trigger policy**: nothing runs on push/PR — everything is `workflow_dispatch` (run on demand) + tag push (`v*`) + monthly schedule. `integration.yml` and acceptance tests provision REAL VMs and are dispatch/schedule-only, never tag-triggered. All scheduled failures auto-create deduplicated `ci-failure` issues.

| Workflow | Trigger |
|----------|---------|
| `ci.yml` (build/vet/test + generated-files-drift + size-map-drift) | dispatch, tag |
| `integration.yml` (real VM smoke) | dispatch only |
| `acceptance.yml` / `acceptance-scheduled.yml` | dispatch / monthly (3rd) |
| `bridge-e2e.yml` (Pulumi bridge) | dispatch, monthly (4th) |
| `zone-benchmarks.yml` | dispatch only |

## Testing

```bash
go test ./provider/ -count=1 -timeout 120s     # unit tests (mocked HTTP)
go vet ./provider/                               # static analysis
golangci-lint run                                # lint (must be clean)
```

Acceptance tests (`*_acc_test.go`) require `SHC_API_KEY` and create real VMs. Run with:
```bash
TF_ACC=1 SHC_API_KEY=shc_live_... go test ./provider/ -run TestAcc -v -timeout 600s
```

## Key lessons (from shc-toolkit)

1. **SHC VMs never reach `provisioning_state: "ready"`** — poll for `service_status == "active" && ip != ""` instead. The `provisioning_state` may stay `"provisioning"` forever.

2. **Dev zone (Cherryvale, KS) — RESOLVED 2026-08-25** — issue #28 (scheduler never assigned IPs) recovered; verified pkg 80 provisions in ~90–100s. If it regresses: `../shc-toolkit/scripts/dev-zone-probe.py`.

3. **debian13-cloud works fine** — earlier "deadlock" diagnosis was wrong. Default template is `debian13-cloud` everywhere.

4. **Nested KVM only on Dev plans (pkg 80–84)** — NVMe/SSD/HDD plans do NOT expose VMX/SVM to guests (`/dev/kvm` absent, `vmx/svm` count=0). Verified empirically 2026-07-20. Users needing nested virtualization (QEMU/KVM-in-VM, Firecracker) must use Dev sizes. The Dev zone recovered 2026-08-25 (issue #28 resolved), so nested-KVM workloads are available again.

5. **Order form IDs are per-line, not per-package**:
   - nvme → 1, ssd → 7, hdd → 3, dev → 11

6. **Credit check fails closed** — if the balance endpoint is unreachable, `CheckCredit` returns an error. Do not silently allow orders when credit state cannot be verified.

7. **`if: always()` for CI cleanup** — GitHub Actions timeout kills cleanup code. Always use unconditional cleanup steps in acceptance test workflows.

8. **CHANGELOG discipline** — every change that adds a feature, fixes a bug, or alters behavior MUST add a CHANGELOG entry in the same commit.

## Cross-repo audits

Mechanical parity: run from shc-toolkit — `python3 ../shc-toolkit/scripts/audit_cross_repo.py` (must be all-pass).

Semantic parity: `../shc-toolkit/docs/cross-repo-audit-prompts.md` contains four AI-agent prompts for comparing the two repos (behavioral parity, lessons-ported, DRY boundaries, live drift smoke test). Run after every SHC API update and before tagging.

## Version scheme

Mirrors shc-toolkit: `<SHC_API_VERSION>.<patch>` (e.g., `2.4.24.2`). Both repos targeting the same API version share the same prefix, making cross-repo alignment immediately visible. The patch number is independent per repo.
