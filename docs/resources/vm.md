# shc_vm

Manages a Sovereign Hybrid Compute VPS instance. The VM is provisioned by submitting an order with the specified package and pricing, then polled until ready.

## Example Usage

```terraform
resource "shc_vm" "test" {
  hostname    = "tf-test"
  package_id  = 81
  pricing_id  = 245
  ssh_key     = "ssh-ed25519 AAAA..."
  auto_cancel = true
}

output "vm_ip" {
  value = shc_vm.test.ip
}

output "vm_service_id" {
  value = shc_vm.test.service_id
}
```

## Argument Reference

| Argument      | Type   | Required | Description |
|---------------|--------|----------|-------------|
| `hostname`    | string | yes      | The hostname for the VPS. Changing this forces replacement. |
| `package_id`  | number | yes      | The SHC package ID (81=Standard, 82=Professional, 83=Business). Changing this forces replacement. |
| `pricing_id`  | number | yes      | The SHC pricing ID (245=Standard, 249=Professional, 253=Business). Changing this forces replacement. |
| `ssh_key`     | string | no       | SSH public key to apply after provisioning. |
| `auto_cancel` | bool   | no       | If `true` (default), schedules end-of-term cancellation so the VPS does not auto-renew. |

## Attribute Reference

| Attribute            | Type   | Computed | Description |
|----------------------|--------|----------|-------------|
| `ip`                 | string | yes      | The primary IP address of the VPS. |
| `service_id`         | string | yes      | The SHC service ID for the VPS. |
| `os_user`            | string | yes      | The default OS user for SSH login (typically `debian`). |
| `status`             | string | yes      | The current service status. |
| `provisioning_state` | string | yes      | The provisioning state (`ready`, `provisioning`, etc.). |

## Import

Not yet supported. Use the `data "shc_vm"` data source to read an existing VM.

## Notes

- VMs are provisioned from the `debian13-cloud` template by default.
- Updates are not supported in place; recreate the resource to change `hostname`, `package_id`, or `pricing_id`.
- On destroy, the VM is cancelled immediately with a refund.
