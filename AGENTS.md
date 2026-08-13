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

2. **Dev zone (Cherryvale, KS) is broken** — VMs on pkg 80–84 never provision (scheduler never assigns IP). This is SHC platform issue #28, unrelated to template or provider. NVMe/SSD/HDD in Katy, TX work fine.

3. **debian13-cloud works fine** — earlier "deadlock" diagnosis was wrong. Default template is `debian13-cloud` everywhere.

4. **Order form IDs are per-line, not per-package**:
   - nvme → 1, ssd → 7, hdd → 3, dev → 11

5. **Credit check fails closed** — if the balance endpoint is unreachable, `CheckCredit` returns an error. Do not silently allow orders when credit state cannot be verified.

6. **`if: always()` for CI cleanup** — GitHub Actions timeout kills cleanup code. Always use unconditional cleanup steps in acceptance test workflows.

7. **CHANGELOG discipline** — every change that adds a feature, fixes a bug, or alters behavior MUST add a CHANGELOG entry in the same commit.

## Version scheme

Independent semver (`v0.4.0`). Does NOT mirror the SHC API version.
