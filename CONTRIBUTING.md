# Contributing to terraform-provider-shc

## Development setup

```bash
git clone https://github.com/Amperstrand/terraform-provider-shc.git
cd terraform-provider-shc
go build .
```

Requires Go 1.25+.

## Running tests

### Unit tests (no API key needed)
```bash
go test ./provider/ -count=1 -timeout 120s
```

### Acceptance tests (requires SHC_API_KEY)
```bash
export SHC_API_KEY="shc_live_..."
export TF_ACC=1
go test ./provider/ -run 'TestAccVMResource_Basic$' -v -timeout 10m
```

Acceptance tests create and destroy real VMs (~$0.01-0.02 each). Always run
individually — the test suite is not designed for parallel execution.

### Linting
```bash
gofmt -l provider/
go vet ./...
```

## Code structure

```
provider/
├── client.go              # SHCClient struct, HTTP infrastructure
├── vm_client.go           # VM lifecycle methods
├── snapshot_client.go     # Snapshot + backup methods
├── firewall_client.go     # Firewall + rDNS methods
├── catalog_client.go      # Catalog, templates, billing methods
├── cloudinit_client.go    # Cloud-init + batch methods
├── vm_resource.go         # VM resource schema + CRUD
├── snapshot_resource.go   # Snapshot resource
├── firewall_resource.go   # Firewall resource
├── rdns_resource.go       # rDNS resource
├── backup_resource.go     # Backup resource
├── *_data_source.go       # Data sources (vm, vms, catalog, etc.)
├── *_function.go          # Provider functions (parse_size, estimate_cost)
├── types.go               # Shared type definitions
├── errors.go              # Structured error diagnostics
├── validators.go          # Input validators
├── sizes.go               # Size name → package/pricing map
├── cost_audit.go          # Cost tracking
└── case_insensitive_type.go # Custom type for semantic equality
```

## Pull request process

1. Fork the repository
2. Create a feature branch (`git checkout -b fix/my-fix`)
3. Run `go build ./...` and `go test ./provider/`
4. Commit with a clear message (see CHANGELOG for style)
5. Open a PR with a description of what changed and why
6. Ensure CI passes

## CHANGELOG discipline

Every PR that adds a feature or fixes a bug should add a CHANGELOG entry
under `[Unreleased]`. Format: [Keep a Changelog](https://keepachangelog.com/).

## Releasing

Releases are currently manual:
```bash
git tag v0.X.0
git push origin v0.X.0
# Cross-compile + gh release create
```

See `docs/registry-submission-checklist.md` for future automated process.

## License

MIT. By contributing, you agree your contributions are licensed under the MIT license.
