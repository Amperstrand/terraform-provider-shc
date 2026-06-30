# Terraform Provider for SHC

Terraform provider for Sovereign Hybrid Compute (SHC) VPS. Manage SHC virtual machines, snapshots, and backups as Terraform infrastructure-as-code.

## Features

- VM lifecycle: create, read, update (via replacement), and delete VPS instances
- Snapshot management: create, read, and delete VPS snapshots
- Backup management: create, read, and delete VPS backups
- SSH key injection: apply a public key to a VPS after provisioning
- Confirmation flow handling: automatically resolves SHC order confirmation requests
- Auto-cancel: optionally schedule end-of-term cancellation so VPS do not auto-renew

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

2. Or pass it explicitly in the provider block (see the usage example below).

The API key is treated as sensitive and will not appear in plan or state output.

## Usage

```hcl
terraform {
  required_providers {
    shc = {
      source = "sovereignhybridcompute/shc"
    }
  }
}

variable "shc_api_key" {
  type      = string
  sensitive = true
}

provider "shc" {
  api_key = var.shc_api_key
}

resource "shc_vm" "test" {
  hostname   = "tf-test"
  package_id = 81
  pricing_id = 245
}

output "vm_ip" {
  value = shc_vm.test.ip
}
```

See [`examples/main.tf`](examples/main.tf) for a more complete example including snapshots.

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
| `package_id`  | number | yes      | The SHC package ID (81=Standard, 82=Professional, 83=Business). Changing this triggers an in-place upgrade. |
| `pricing_id`  | number | yes      | The SHC pricing ID (245=Standard, 249=Professional, 253=Business). Changing this triggers an in-place upgrade. |
| `ssh_key`     | string | no       | SSH public key to apply after provisioning. |
| `auto_cancel` | bool   | no       | If `true` (default), schedules end-of-term cancellation so the VPS does not auto-renew. |
| `power_state` | string | no       | The desired power state: `running` (default) or `stopped`. Changing this triggers a start/stop action without replacing the VM. |

| Attribute            | Type   | Computed | Description |
|----------------------|--------|----------|-------------|
| `ip`                 | string | yes      | The primary IP address of the VPS. |
| `service_id`         | string | yes      | The SHC service ID for the VPS. |
| `os_user`            | string | yes      | The default OS user for SSH login (typically `debian`). |
| `status`             | string | yes      | The current service status. |
| `provisioning_state` | string | yes      | The provisioning state (`ready`, `provisioning`, etc.). |

### Upgrading a VM

Changing `package_id` and `pricing_id` on an existing VM triggers an in-place upgrade
instead of destroy/recreate. The upgrade is queued — it creates a prorated invoice and
the VM is resized after payment.

Only upgrades (more CPU/RAM/disk) are supported. Disk-reducing changes are rejected by
the API with a 422 error.

```hcl
# Upgrade from Standard to Professional
resource "shc_vm" "web" {
  hostname   = "web-server"
  package_id = 82  # was 81
  pricing_id = 249 # was 245
}
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

Import with `terraform import shc_rdns.example "service_id:ip"`.

## Data Sources

- `shc_vm` - Reads an existing VPS by service ID. Requires `service_id` and exports `hostname`, `ip`, `os_user`, `status`, and `provisioning_state`.

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

**Get SHC VPS**: [Sovereign Hybrid Compute](https://blesta.sovereignhybridcompute.com/order/forms/a/lecture-mushroom-lunar) — bitcoin-native VPS hosting
