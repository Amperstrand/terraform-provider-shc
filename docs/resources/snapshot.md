# shc_snapshot

Manages a snapshot of an SHC VPS instance.

## Example Usage

```terraform
resource "shc_snapshot" "daily" {
  service_id = shc_vm.test.service_id
  name       = "daily-snapshot"
}
```

## Argument Reference

| Argument     | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `service_id` | string | yes      | The SHC service ID of the VPS to snapshot. Changing this forces replacement. |
| `name`       | string | no       | A name for the snapshot. Changing this forces replacement. |

## Attribute Reference

| Attribute      | Type   | Computed | Description |
|----------------|--------|----------|-------------|
| `snapshot_id`  | string | yes      | The ID of the created snapshot. |
| `status`       | string | yes      | The status of the snapshot. |

## Notes

- Snapshots cannot be updated; recreate the resource to change its name.
- On destroy, the snapshot is permanently deleted.
