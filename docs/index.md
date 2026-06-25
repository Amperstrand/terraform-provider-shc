# SHC Terraform Provider

The Sovereign Hybrid Compute (SHC) Terraform provider allows you to manage VPS instances, snapshots, and backups on the SHC platform using Terraform.

## Example Usage

```terraform
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

resource "shc_vm" "test" {
  hostname    = "tf-test"
  package_id  = 81
  pricing_id  = 245
  auto_cancel = true
}

output "vm_ip" {
  value = shc_vm.test.ip
}

output "vm_service_id" {
  value = shc_vm.test.service_id
}
```

## Authentication

The provider authenticates using a Bearer token (API key). Set the `api_key` argument in the provider block, or via the `SHC_API_KEY` environment variable.

## Provider Configuration

| Argument   | Type     | Required | Sensitive | Description |
|------------|----------|----------|-----------|-------------|
| `api_key`  | string   | yes      | yes       | The SHC API key for authentication. |
| `endpoint` | string   | no       | no        | The SHC API base URL. Defaults to `https://blesta.sovereignhybridcompute.com/user-api/v2`. |

## Resources

- [`shc_vm`](resources/vm.md) — Manages a VPS instance.
- [`shc_snapshot`](resources/snapshot.md) — Manages a VPS snapshot.
- [`shc_backup`](resources/backup.md) — Manages a VPS backup.

## Data Sources

- `shc_vm` — Reads an existing VPS by service ID.
