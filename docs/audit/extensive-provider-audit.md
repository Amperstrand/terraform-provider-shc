> **⚠️ Historical document (2026-08-07).** This audit reflects the pre-v0.4.0 codebase.
> Many issues identified here have been resolved. See `AGENTS.md` for current guidance.

# Extensive Provider Audit: Best Practices, Patterns, and Gaps

**Date**: 2026-08-07
**Scope**: terraform-provider-shc vs production-grade providers (hetzner, linode, vultr, digitalocean)
**Method**: Source code analysis via GitHub API, HashiCorp official docs, web research

---

## Executive Summary

Our provider is **functional and well-architected** for its scope, but has significant gaps in testing depth, error handling sophistication, and logging compared to production-grade providers. The provider uses a custom HTTP client while ALL compared providers use `hashicorp/go-retryablehttp` via their SDKs.

---

## 1. Library Comparison

### HTTP Client

| Provider | HTTP Library | Retry Library |
|----------|-------------|---------------|
| **Hetzner** | `hcloud-go/v2` (own SDK) | `go-retryablehttp` (indirect) |
| **Linode** | `linodego/v2` (own SDK) | `go-retryablehttp` (indirect) |
| **Vultr** | `govultr/v3` (own SDK) | `go-retryablehttp` (indirect) |
| **DigitalOcean** | `godo` (own SDK) | `go-retryablehttp` (indirect) |
| **SHC (ours)** | Custom `SHCClient` with `net/http` | Custom retry (just added) |

**Finding**: ALL production providers use `hashicorp/go-retryablehttp` for HTTP retry. We just built our own retry logic (which works), but the industry standard is to use this library. It handles:
- Configurable retry policies (retry count, backoff, retryable status codes)
- Context-aware cancellation
- Request body caching for safe retries
- Standard logging integration

**Recommendation**: **MEDIUM priority** — Our custom retry works correctly. Adopting `go-retryablehttp` would reduce maintenance burden and align with industry practice, but is not urgent. File as v0.3.0 work.

### Logging

| Provider | Logging Approach |
|----------|-----------------|
| **Hetzner** | `tflogutil` package wrapping `tflog` |
| **Linode** | `tflog` throughout |
| **Vultr** | `tflog` |
| **SHC (ours)** | **None** (no structured logging) |

**Finding**: Every production provider uses `tflog` for structured logging. This enables `TF_LOG=DEBUG` debugging, which is critical for users troubleshooting provider issues. Our provider has zero logging.

**Recommendation**: **HIGH priority** — Add `tflog.Info(ctx, ...)` / `tflog.Debug(ctx, ...)` calls to all CRUD methods. This is the #1 most impactful improvement for user supportability.

---

## 2. Testing Comparison

### Test File Count

| Provider | Test Files | Acceptance Tests | Unit Tests |
|----------|-----------|-----------------|------------|
| **Hetzner** | ~50+ | ~200+ | ~300+ |
| **Linode** | ~100+ | ~500+ | ~200+ |
| **Vultr** | 71 | ~150+ | ~100+ |
| **SHC (ours)** | **8** | **4** | **70+** |

### Missing Test Patterns (per HashiCorp testing docs)

Our provider is missing these standard test patterns:

1. **Update tests** — Create resource, change config, verify update applied correctly
   - We need: size upgrade test, power_state change test, term change test
   
2. **Error/ExpectError tests** — Verify invalid configs produce expected errors
   - We need: invalid hostname validator test, invalid size validator test, missing required field test
   
3. **CheckDestroy function** — Verify resources are actually deleted after test
   - We have: **NONE** — tests don't verify cleanup
   - Every production provider has CheckDestroy per resource
   
4. **Random test names** — Use `acctest.RandStringFromCharSet` for unique hostnames
   - We use: Fixed hostnames (`tf-acc-basic`) → collision risk in parallel CI
   
5. **Multi-step test sequences** — Create → Update → Import in one TestCase
   - We have: Single-step tests only
   
6. **Regression tests** — Named after bug reports, verify fixes don't regress
   - We need: Regression test for the order_form_id bug, provisioning_state bug, etc.

### CPU/RAM Edge Case Testing

Currently **NOT tested**:
- What happens when user provides `cpu=0`? (validator rejects, but no acceptance test verifies)
- What happens when user provides `ram_mb=1`? (too small for any plan)
- What happens when user provides `disk_gb=1`? (too small)
- What happens when user provides conflicting `size` and `package_id`?
- What happens with maximum values (`cpu=16, ram_mb=65536`)?
- What happens with size/plan upgrade then downgrade?

**Recommendation**: **HIGH priority** — Add ExpectError acceptance tests for invalid CPU/RAM/disk combinations. Add unit tests for boundary values in validators.

---

## 3. Error Handling Comparison

### Hetzner's Approach (Gold Standard)

Hetzner has a dedicated `hcloudutil/error_framework.go` with:
- Typed API error parsing (errors.As into `hcloud.Error`)
- Field-level error diagnostics ("Field: name, Messages: ...")
- Status code inclusion in diagnostics
- Error code categorization (invalid_input, not_found, rate_limit, etc.)

### Our Approach

We use generic `fmt.Errorf("...: %w", err)` wrapping. API errors are returned as raw strings.

**Gap**: When the SHC API returns a validation error (400), our provider shows the raw JSON response. Production providers parse the error into structured diagnostics with field-level detail.

**Recommendation**: **MEDIUM priority** — Parse SHC API error responses into structured diagnostics. Model on Hetzner's `error_framework.go`.

---

## 4. Schema Design Audit

### What We Do Well ✅
- `RequiresReplace()` on `hostname` (immutable field)
- Custom `packageIDUpgrade` plan modifier (in-place upgrade)
- `Sensitive: true` on `ssh_key` and `api_key`
- `Computed` flags correctly set (after our fix)
- Input validators (hostname RFC 1123, size regex, positive int)
- State upgrader (v0→v1)

### What We're Missing ⚠️
1. **`UseStateForUnknown()` plan modifiers** — Production providers use this on computed fields to prevent spurious diffs during updates. We don't use it anywhere.
2. **`stringvalidator.LengthBetween()` on hostname** — We validate hostname format but not length limits (63 chars per RFC 1123)
3. **`RequiresReplaceIf`** on template — Template changes should trigger replace (reinstall), not in-place update. Currently template changes produce a perpetual diff because Update() doesn't handle them.
4. **Default values via `Computed` + `Default`** — Some fields like `auto_cancel` have defaults, but others don't where they should (e.g., `power_state` has a default but no explicit `Computed` interaction is tested)

---

## 5. Architecture Comparison

### Hetzner's Package Structure
```
internal/
├── control/          # Retry logic (Retry, AbortRetry, ExponentialBackoff)
├── convutil/         # Type conversion utilities
├── datasourceutil/   # Data source helpers
├── hcloudutil/       # Error handling, API client, labels, list helpers
├── merge/            # Map merge utilities
├── resourceutil/     # Resource helpers
├── tflogutil/        # Logging wrapper
└── experimental/     # Experimental features
```

### Our Structure
```
provider/
├── client.go         # Everything HTTP + business logic (1412 lines)
├── vm_resource.go    # VM resource (677 lines)
├── types.go          # Type definitions
├── validators.go     # Validators
├── plan_modifiers.go # Plan modifiers
├── sizes.go          # Size map
├── cost_audit.go     # Cost tracking
└── *_test.go         # Tests
```

**Finding**: Our `client.go` at 1412 lines is a monolith. Production providers split client logic from business logic from provider logic.

**Recommendation**: **LOW priority** — Restructure into `internal/client/`, `internal/provider/`, `internal/validator/` packages. Not urgent but would improve maintainability.

---

## 6. Priority Action Items

### Immediate (v0.2.0 — blocking release quality)
1. **Add `tflog` logging to CRUD methods** — Most impactful for supportability
2. **Add `CheckDestroy` to acceptance tests** — Proves resources are actually cleaned up
3. **Use random hostnames in acceptance tests** — Avoid CI collisions
4. **Add at least one ExpectError test** — Verify validators work in real Terraform

### Short-term (v0.3.0)
5. **Adopt `go-retryablehttp`** — Replace custom retry with industry standard
6. **Add update acceptance tests** — Size upgrade, power_state change, term change
7. **Parse API errors into structured diagnostics** — Model on Hetzner's error_framework.go
8. **Add `UseStateForUnknown()` plan modifiers** — Prevent spurious diffs
9. **Add `RequiresReplaceIf` on template** — Proper replace-on-template-change behavior
10. **Add CPU/RAM edge case tests** — Boundary values, invalid combos

### Long-term (v1.0.0)
11. **Restructure into internal/ packages** — Split client.go monolith
12. **Add regression tests for all fixed bugs** — order_form_id, provisioning_state, etc.
13. **Add test sweepers** — Automated cleanup of leaked test resources
14. **Consider SDK extraction** — Extract SHCClient into a separate Go module (like hcloud-go, linodego)

---

## 7. What We Do Better Than Average

For a fair assessment, here's where our provider is already strong:

1. ✅ **HTTP retry with jitter** — We have this (just added); many small providers don't
2. ✅ **Confirmation flow handling** — Our 409→confirm flow is well-implemented
3. ✅ **Cost tracking** — CostTracker with proration, refund matching, mismatch detection
4. ✅ **Schema versioning** — StateUpgrader from day one
5. ✅ **Context propagation** — Proper context.Context throughout
6. ✅ **Error wrapping** — `%w` verb used consistently
7. ✅ **Size abstraction** — `size="nvme-2c-8gb"` is a great UX improvement over raw IDs
8. ✅ **Idempotency-Key** — Just added; many providers don't bother
9. ✅ **Cross-zone order_form_id resolution** — Dynamic catalog lookup; no hardcoded values

---

## References

- [HashiCorp Provider Design Principles](https://developer.hashicorp.com/terraform/plugin/best-practices/hashicorp-provider-design-principles)
- [HashiCorp Testing Patterns](https://developer.hashicorp.com/terraform/plugin/testing/testing-patterns)
- [hashicorp/go-retryablehttp](https://github.com/hashicorp/go-retryablehttp)
- [Hetzner error_framework.go](https://github.com/hetznercloud/terraform-provider-hcloud/blob/main/internal/util/hcloudutil/error_framework.go)
- [Hetzner retry logic](https://github.com/hetznercloud/terraform-provider-hcloud/blob/main/internal/util/control/retry.go)
