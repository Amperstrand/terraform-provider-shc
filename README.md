# Terraform Provider for SHC

Terraform provider for Sovereign Hybrid Compute (SHC) VPS. Manage SHC virtual machines, snapshots, backups, firewall rules, and reverse DNS as Terraform infrastructure-as-code.

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
  size     = "standard"
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
- Size abstraction: pick a plan by name (`size = "standard"`) instead of numeric IDs
- In-place upgrade: change `size` or `package_id`/`pricing_id` to upgrade without recreate
- Power management: start/stop a VM with `power_state = "stopped"`
- NoDNS: auto-publish a `.nodns.shop` or `.dns4sats.xyz` hostname via Nostr
- Firewall: manage per-VM firewall rules (`shc_firewall_rule`)
- Reverse DNS: manage PTR records (`shc_rdns`)
- Snapshots and backups: create, read, restore, and delete
- SSH key injection: apply a public key to a VPS after provisioning
- Confirmation flow handling: automatically resolves SHC order confirmation requests
- Auto-cancel: optionally schedule end-of-term cancellation so VPS do not auto-renew
- Credit safety: pre-checks account credit before ordering to prevent surprise billing
- Data sources: browse the catalog, templates, and machine types
- Import: bring existing VMs under Terraform management

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

Manages a Sovereign Hybrid Compute VPS instance. The VM is provisioned by submitting an order with the specified package and pricing, then polled until it reaches the `ready` state.

| Argument      | Type   | Required | Description |
|---------------|--------|----------|-------------|
| `hostname`    | string | yes      | The hostname for the VPS. Changing this forces replacement. |
| `size`        | string | no       | Named size: `starter`, `standard`, `professional`, `business`, `enterprise` (NVMe), or `dev-starter`, `dev-standard`, `dev-professional`, `dev-business`, `dev-enterprise` (Dev VPS). Takes precedence over `package_id`/`pricing_id`. Changing this triggers an in-place upgrade. |
| `package_id`  | number | no       | The SHC package ID. Required if `size` is not set. Changing this triggers an in-place upgrade. |
| `pricing_id`  | number | no       | The SHC pricing ID. Required if `size` is not set. Changing this triggers an in-place upgrade. |
| `ssh_key`     | string | no       | SSH public key to apply after provisioning. |
| `auto_cancel` | bool   | no       | If `true` (default), schedules end-of-term cancellation so the VPS does not auto-renew. |
| `power_state` | string | no       | Desired power state: `running` (default) or `stopped`. Changing this triggers a start/stop without replacing the VM. |
| `nodns`       | bool   | no       | If `true`, auto-publishes a NoDNS record pointing to the VM's IP after provisioning. Requires `python3` + `shc-toolkit` on the runner. |
| `nodns_zone`  | string | no       | NoDNS zone: `nodns.shop` (default) or `dns4sats.xyz`. Only used when `nodns = true`. |

| Attribute            | Type   | Computed | Description |
|----------------------|--------|----------|-------------|
| `ip`                 | string | yes      | The primary IP address of the VPS. |
| `service_id`         | string | yes      | The SHC service ID for the VPS. |
| `os_user`            | string | yes      | The default OS user for SSH login (typically `debian`). |
| `status`             | string | yes      | The current service status. |
| `provisioning_state` | string | yes      | The provisioning state (`ready`, `provisioning`, etc.). |
| `fqdn`               | string | yes      | NoDNS FQDN assigned to the VM (e.g. `npub1abc.nodns.shop`). Only set when `nodns = true`. |
| `nodns_nsec`         | string | yes      | Nostr secret key (nsec) for the NoDNS record. **Sensitive.** Store securely; needed to update the record later. |

#### Size abstraction (recommended)

Instead of numeric `package_id` and `pricing_id`, use `size` for a human-readable plan name:

```hcl
resource "shc_vm" "web" {
  hostname = "web"
  size     = "standard"
}
```

Available sizes: `starter`, `standard`, `professional`, `business`, `enterprise` (NVMe);
`dev-starter`, `dev-standard`, `dev-professional`, `dev-business`, `dev-enterprise` (Dev VPS).

#### In-place upgrade

Changing `size` (or `package_id`/`pricing_id`) on an existing VM triggers an in-place upgrade instead of destroy/recreate. The upgrade is queued -- it creates a prorated invoice and the VM is resized after payment.

Only upgrades (more CPU/RAM/disk) are supported. Disk-reducing changes are rejected by the API with a 422 error.

```hcl
resource "shc_vm" "web" {
  hostname = "web-server"
  size     = "professional"  # was "standard"
}
```

#### NoDNS hostname

Set `nodns = true` to automatically get a `.nodns.shop` (or `.dns4sats.xyz`) domain pointing to the VM's IP. The provider publishes a kind 11111 Nostr event via the Python `shc-toolkit`. The resulting FQDN and nsec secret key are exposed as outputs.

Requires `python3` and `shc-toolkit` (with `nostr-sdk`) on the Terraform runner:

```sh
pip install shc-toolkit[nostr]
```

```hcl
resource "shc_vm" "web" {
  hostname   = "web-server"
  size       = "standard"
  nodns      = true
  nodns_zone = "dns4sats.xyz"
}

output "vm_fqdn" {
  value = shc_vm.web.fqdn
}

output "vm_nsec" {
  value     = shc_vm.web.nodns_nsec
  sensitive = true
}
```

#### Power management

Control whether a VM is running or stopped:

```hcl
resource "shc_vm" "db" {
  hostname    = "database"
  size        = "standard"
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

## Known Limitations

- **Snapshots & backups not available on Dev VPS plans**: Dev VPS plans (pkg 80-84) lack the storage infrastructure for snapshots and backups. The `shc_snapshot` and `shc_backup` resources will fail with `upstream_failure` on these plans. Use NVMe/SSD/HDD VPS plans (pkg 23+) for snapshot and backup support. All other API features (firewall, rDNS, ISO, console, metrics) work on both plan types.

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

**Get SHC VPS**: [Sovereign Hybrid Compute](https://blesta.sovereignhybridcompute.com/order/forms/a/lecture-mushroom-lunar) -- bitcoin-native VPS hosting
