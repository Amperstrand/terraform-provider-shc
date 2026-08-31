# Terraform Provider for SHC

Terraform provider for Sovereign Hybrid Compute (SHC) VPS. Manage SHC virtual machines, snapshots, backups, firewall rules, and reverse DNS as Terraform infrastructure-as-code.

**Works with Pulumi too** — no separate provider needed. See [Pulumi via Terraform Bridge](#pulumi-via-terraform-bridge) below.

## Related Projects

- [shc-toolkit](https://github.com/Amperstrand/shc-toolkit) — Python client, CLI, and provisioning toolkit for SHC (v2.4.24+)
- [shc-pulumi](https://github.com/Amperstrand/shc-pulumi) — ⛔ Deprecated. Use this provider via the Pulumi TF Bridge instead.

## Quick Start

The simplest possible configuration -- one VM on the standard plan:

```hcl
terraform {
  required_providers {
    shc = {
      source = "sovereignhybridcompute/shc"
    }
  }
}

provider "shc" {
  api_key = var.shc_api_key
}

variable "shc_api_key" {
  type      = string
  sensitive = true
}

resource "shc_vm" "web" {
  hostname = "web"
  size     = "nvme-2c-8gb"
}

output "vm_ip" {
  value = shc_vm.web.ip
}
```

```sh
export SHC_API_KEY="shc_live_..."
terraform init
terraform apply
```

## Features

- VM lifecycle: create, read, update (in-place upgrade), and delete VPS instances
- Size abstraction: pick a plan by spec-encoding name (`size = "nvme-2c-8gb"`) instead of numeric IDs
- In-place upgrade: change `size` or `package_id`/`pricing_id` to upgrade without recreate
- Power management: start/stop a VM with `power_state = "stopped"`
- **Billing term management**: change `term` to switch billing period (v2.4.6)
- Firewall: manage per-VM firewall rules (`shc_firewall_rule`)
- Reverse DNS: manage PTR records (`shc_rdns`)
- Snapshots and backups: create, read, restore, and delete
- SSH key injection: apply a public key to a VPS after provisioning
- Confirmation flow handling: automatically resolves SHC order confirmation requests
- Auto-cancel: optionally schedule end-of-term cancellation so VPS do not auto-renew
- Credit safety: pre-checks account credit before ordering to prevent surprise billing
- HTTP retry: automatic retry on 429/503 with exponential backoff + jitter (via go-retryablehttp)
- Input validation: hostname (RFC 1123), size format, positive-integer on CPU/RAM/disk
- Semantic equality: firewall action/protocol are case-insensitive (accept = ACCEPT)
- Provider-defined function: `provider::shc::parse_size("nvme-2c-8gb")` returns CPU/RAM/package/pricing
- Write-only `ssh_key`: SSH keys passed at apply time but never stored in state
- Structured error diagnostics: SHC API errors parsed into field-level detail
- tflog: structured logging on all resources (`TF_LOG=DEBUG`)
- Data sources: browse the catalog, templates, and machine types
- Import: bring existing VMs under Terraform management
- Schema versioning: `SchemaVersion: 1` with state upgrader for future breaking changes

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/doc/install) >= 1.25 (to build the provider from source)

## Installation

### From source

Clone the repository and build the provider binary:

```sh
git clone https://github.com/Amperstrand/terraform-provider-shc.git
cd terraform-provider-shc
make build
```

Or build directly with Go:

```sh
go build -o terraform-provider-shc
```

Install the binary into the local Terraform plugin directory:

```sh
make install
```

## Authentication

The provider authenticates against the SHC API using a Bearer token (API key). Provide the key in one of two ways:

1. Set the `SHC_API_KEY` environment variable:

   ```sh
   export SHC_API_KEY="your-api-key"
   ```

2. Or pass it explicitly in the provider block (see the Quick Start example).

The API key is treated as sensitive and will not appear in plan or state output.

## Provider Configuration

| Argument   | Type   | Required | Sensitive | Description |
|------------|--------|----------|-----------|-------------|
| `api_key`  | string | yes      | yes       | The SHC API key for authentication. |
| `endpoint` | string | no       | no        | The SHC API base URL. Defaults to `https://blesta.sovereignhybridcompute.com/user-api/v2`. |

## Resources

### shc_vm

Manages a Sovereign Hybrid Compute VPS instance. The VM is provisioned by submitting an order with the specified package and pricing, then polled until `service_status == "active"` and an IP is assigned (SHC VMs may report `provisioning_state: "provisioning"` indefinitely — see Known Limitations).

| Argument      | Type   | Required | Description |
|---------------|--------|----------|-------------|
| `hostname`    | string | yes      | The hostname for the VPS. Changing this forces replacement. |
| `size`        | string | no       | Spec-encoding size name: `{line}-{cpu}c-{ram}gb` (e.g. `nvme-2c-8gb`, `hdd-1c-4gb`, `ssd-4c-16gb`, `dev-8c-32gb`). Takes precedence over `package_id`/`pricing_id`. Changing this triggers an in-place upgrade. |
| `package_id`  | number | no       | The SHC package ID. Required if `size` is not set. Changing this triggers an in-place upgrade. |
| `pricing_id`  | number | no       | The SHC pricing ID. Required if `size` is not set. Changing this triggers an in-place upgrade. |
| `ssh_key`     | string | no       | SSH public key to apply after provisioning. |
| `auto_cancel` | bool   | no       | If `true` (default), schedules end-of-term cancellation so the VPS does not auto-renew. |
| `power_state` | string | no       | Desired power state: `running` (default) or `stopped`. Changing this triggers a start/stop without replacing the VM. |
| `term`        | number | no       | Billing term (pricing_id of the desired term, e.g. 56=daily, 58=monthly). If unset, the API default (monthly) is used. |

| Attribute            | Type   | Computed | Description |
|----------------------|--------|----------|-------------|
| `ip`                 | string | yes      | The primary IP address of the VPS. |
| `service_id`         | string | yes      | The SHC service ID for the VPS. |
| `os_user`            | string | yes      | The default OS user for SSH login (typically `debian`). |
| `status`             | string | yes      | The current service status. |
| `provisioning_state` | string | yes      | The provisioning state (`ready`, `provisioning`, etc.). |

#### Size abstraction (recommended)

Instead of numeric `package_id` and `pricing_id`, use `size` for a human-readable plan name:

```hcl
resource "shc_vm" "web" {
  hostname = "web"
  size     = "nvme-2c-8gb"
}
```

Available sizes: `nvme-1c-4gb`, `nvme-2c-8gb`, `nvme-4c-16gb`, `nvme-8c-32gb`, `nvme-16c-64gb`, `ssd-1c-4gb`, `ssd-2c-8gb`, `ssd-4c-16gb`, `ssd-8c-32gb`, `ssd-16c-64gb`, `hdd-1c-4gb`, `hdd-2c-8gb`, `hdd-4c-16gb`, `hdd-8c-32gb`, `hdd-16c-64gb`, `dev-1c-4gb`, `dev-2c-8gb`, `dev-4c-16gb`, `dev-8c-32gb`, `dev-16c-64gb` (spec-encoding).

#### In-place upgrade

Changing `size` (or `package_id`/`pricing_id`) on an existing VM triggers an in-place upgrade instead of destroy/recreate. The upgrade is queued -- it creates a prorated invoice and the VM is resized after payment.

Only upgrades (more CPU/RAM/disk) are supported. Disk-reducing changes are rejected by the API with a 422 error.

```hcl
resource "shc_vm" "web" {
  hostname = "web-server"
  size     = "nvme-4c-16gb"  # was "nvme-2c-8gb"
}
```

#### Power management

Control whether a VM is running or stopped:

```hcl
resource "shc_vm" "db" {
  hostname    = "database"
  size        = "nvme-2c-8gb"
  power_state = "stopped"
}
```

Changing `power_state` triggers a start/stop action without replacing the VM.

#### Credit safety

Before submitting an order, the provider checks that your account has at least $0.50 of available credit (the cheapest daily plan). This prevents surprise billing from an order that would create an unpaid invoice. If credit is insufficient, `terraform apply` fails fast with a link to add credit.

The check fails open: if the billing endpoint is temporarily unreachable, ordering proceeds so that transient outages do not block provisioning.

#### Import

Bring an existing VM under Terraform management by its service ID:

```sh
terraform import shc_vm.web 123
```

### shc_snapshot

Manages a snapshot of an SHC VPS instance.

| Argument     | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `service_id` | string | yes      | The SHC service ID of the VPS to snapshot. Changing this forces replacement. |
| `name`       | string | no       | A name for the snapshot. Changing this forces replacement. |

| Attribute     | Type   | Computed | Description |
|---------------|--------|----------|-------------|
| `snapshot_id` | string | yes      | The ID of the created snapshot. |
| `status`      | string | yes      | The status of the snapshot. |

Import with `terraform import shc_snapshot.example "service_id:snapshot_id"`.

### shc_backup

Manages a backup of an SHC VPS instance.

| Argument     | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `service_id` | string | yes      | The SHC service ID of the VPS to back up. Changing this forces replacement. |
| `name`       | string | no       | A name for the backup. Changing this forces replacement. |

| Attribute   | Type   | Computed | Description |
|-------------|--------|----------|-------------|
| `backup_id` | string | yes      | The ID of the created backup. |
| `status`    | string | yes      | The status of the backup. |

Import with `terraform import shc_backup.example "service_id:backup_id"`.

### shc_firewall_rule

Manages a firewall rule on an SHC VPS instance. Rules are identified by their position in the chain.

| Argument     | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `service_id` | string | yes      | The SHC service ID of the VPS. Changing this forces replacement. |
| `action`     | string | no       | The firewall action: `accept` (default), `drop`, or `reject`. |
| `protocol`   | string | no       | The protocol: `tcp` (default), `udp`, or `icmp`. |
| `port`       | string | no       | The destination port (e.g. `22`, `80,443`). |
| `source`     | string | no       | The source CIDR. Defaults to `0.0.0.0/0`. |
| `direction`  | string | no       | The direction: `in` (default) or `out`. |
| `name`       | string | no       | A label or comment for the rule. |

| Attribute  | Type   | Computed | Description |
|------------|--------|----------|-------------|
| `position` | number | yes      | The position of the rule in the chain. |

```hcl
resource "shc_firewall_rule" "allow_https" {
  service_id = shc_vm.web.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "443"
  source     = "0.0.0.0/0"
  name       = "allow-https"
}
```

Import with `terraform import shc_firewall_rule.example "service_id:position"`.

### shc_rdns

Manages reverse DNS (PTR record) for an IP address on an SHC VPS instance.

| Argument     | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `service_id` | string | yes      | The SHC service ID of the VPS. Changing this forces replacement. |
| `ip`         | string | yes      | The IP address to set reverse DNS for. Changing this forces replacement. |
| `hostname`   | string | yes      | The FQDN to set as the PTR record. |

| Attribute | Type   | Computed | Description |
|-----------|--------|----------|-------------|
| `job_id`  | string | yes      | The async job ID for the rDNS operation. |

```hcl
resource "shc_rdns" "mail" {
  service_id = shc_vm.web.service_id
  ip         = shc_vm.web.ip
  hostname   = "mail.example.com"
}
```

Import with `terraform import shc_rdns.example "service_id:ip"`.

## Data Sources

### shc_catalog

Fetches the SHC ordering catalog, listing available VPS packages and their resource specifications (CPU, memory, disk).

```hcl
data "shc_catalog" "current" {}

output "packages" {
  value = data.shc_catalog.current.packages
}
```

### shc_templates

Fetches the list of available OS templates for SHC VPS instances (name, family, arch, status).

```hcl
data "shc_templates" "available" {}

output "template_names" {
  value = data.shc_templates.available.templates[*].name
}
```

### shc_machine_types

Fetches the SHC catalog with resource specs and pricing (daily, weekly, monthly) per machine type.

```hcl
data "shc_machine_types" "all" {}

output "machine_types" {
  value = data.shc_machine_types.all.machine_types
}
```

### shc_vm (data source)

Reads an existing VPS by service ID. Requires `service_id` and exports `hostname`, `ip`, `os_user`, `status`, and `provisioning_state`.

```hcl
data "shc_vm" "existing" {
  service_id = "123"
}
```

### shc_vms (data source)

Lists all VMs on the account, optionally filtered by `status` (exact service status), `zone` (`katy` — NVMe/SSD/HDD lines, or `cherryvale` — Dev VPS), or `package` (case-insensitive substring of the package name). Useful for inventory and cost analysis.

```hcl
# All active Dev-zone VMs
data "shc_vms" "dev_active" {
  zone   = "cherryvale"
  status = "active"
}

output "dev_hostnames" {
  value = data.shc_vms.dev_active.vms[*].hostname
}
```

Each entry exposes `service_id`, `hostname`, `status`, `provisioning_state`, `ip`, and `package`.

## Pulumi via Terraform Bridge

This provider works natively with [Pulumi](https://www.pulumi.com/) via the "Any Terraform Provider" feature — no separate Pulumi provider needed. The native `shc-pulumi` Python provider is deprecated in favor of this path.

### Quick Start

```bash
# 1. Build the Terraform provider
git clone https://github.com/Amperstrand/terraform-provider-shc
cd terraform-provider-shc
go build -o terraform-provider-shc .

# 2. Create a Pulumi project
mkdir my-pulumi-project && cd my-pulumi-project
pulumi new python

# 3. Generate a Pulumi SDK from the TF provider
pulumi package add terraform-provider ./terraform-provider-shc --language python

# 4. Use it in your Pulumi program
```

```python
import shc
import pulumi

config = pulumi.Config()
provider = shc.Provider("shc", api_key=config.require_secret("shc_api_key"))

vm = shc.Vm("web",
    hostname="web",
    size="nvme-2c-8gb",
    opts=pulumi.ResourceOptions(provider=provider),
)

pulumi.export("ip", vm.ip)
```

```bash
export SHC_API_KEY="shc_live_..."
pulumi up
```

→ **[Full migration guide from shc-pulumi](https://github.com/Amperstrand/shc-pulumi/blob/main/MIGRATION-TO-BRIDGE.md)**

Lifecycle semantics are inherited from the provider unchanged through the
bridge: `pulumi destroy` → `CancelVM(immediate)` (billing ends, refund);
`power_state` is a mutable property (`PowerState`) whose `"stopped"` value
pauses without destroying — and still bills. See
[lifecycle-alignment.md](docs/lifecycle-alignment.md).

### Why the bridge?

| | TF Provider + Bridge | Deprecated shc-pulumi |
|---|---|---|
| Features | HTTP retry, input validators, idempotency keys, acceptance-tested CRUD | Subset, mocked tests only |
| Maintenance | Auto-syncs with this repo | Manual, archived |
| Resources | All (VM, snapshot, backup, firewall, rDNS) | VM + snapshot only |

## Lifecycle Semantics

Aligned with AWS/GCP IaC conventions, with SHC's divergences documented and
reasoned. The short version:

- **`terraform destroy` terminates (SHC: *cancels*, immediate + prorated refund)** — never stops. Billing ends.
- **`power_state = "stopped"` pauses without destroying** (GCP `desired_status` pattern) — but unlike AWS/GCP, a stopped SHC VM **keeps billing its full daily price**. Stop is not a cost control here.
- **Deletion protection** = SHC's server-side confirm-gate on destructive ops + Terraform-native `prevent_destroy`. No extra attribute needed.
- **Ephemeral by default**: `auto_cancel = true` (destroy-at-term) — the inverse of cloud renewal-by-default, chosen deliberately.
- **Orphan hygiene**: daily reaper (AWS-sweeper equivalent) + opt-in on-VM self-destruct timer.

→ **[Full mapping table and design reasoning: `docs/lifecycle-alignment.md`](docs/lifecycle-alignment.md)**

## Known Limitations

- **Dev zone (Cherryvale, KS) — RESOLVED**: Dev VPS provisioning (issue #28) recovered and verified 2026-08-25 (pkg 80 in ~90–100s, debian12 and debian13). Nested-KVM workloads (Dev plans only) are available again.
- **Snapshot/backup limit**: All VPS plans (including Dev VPS) support 1 snapshot and 1 backup concurrently.
- **Provisioning state**: SHC VMs may report `provisioning_state: "provisioning"` indefinitely even when fully operational. The provider detects readiness via `service_status == "active" && ip assigned`.

## Distribution

This provider is distributed via GitHub Releases with pre-compiled binaries for Linux and macOS (amd64/arm64). Terraform Registry submission is under consideration for a future release — see the [v0.2.0 release notes](https://github.com/Amperstrand/terraform-provider-shc/releases) for download URLs and SHA256 checksums.

## Development

```sh
make build    # build the provider binary
make fmt      # format all Go source
make vet      # run go vet
make test     # run the test suite
make tidy     # run go mod tidy
make clean    # remove the built binary
```

## License

MIT

---

**Get SHC VPS**: [Sovereign Hybrid Compute](https://blesta.sovereignhybridcompute.com/order/forms/a/lecture-mushroom-lunar) — bitcoin-native VPS hosting

> **Disclosure**: The SHC link above is an affiliate link. If you sign up through it, we receive a **5% recurring commission** (grandfathered rate) on your spending, at no extra cost to you.
