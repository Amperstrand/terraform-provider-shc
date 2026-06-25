# shc_backup

Manages a backup of an SHC VPS instance.

## Example Usage

```terraform
resource "shc_backup" "weekly" {
  service_id = shc_vm.test.service_id
  name       = "weekly-backup"
}
```

## Argument Reference

| Argument     | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `service_id` | string | yes      | The SHC service ID of the VPS to back up. Changing this forces replacement. |
| `name`       | string | no       | A name for the backup. Changing this forces replacement. |

## Attribute Reference

| Attribute   | Type   | Computed | Description |
|-------------|--------|----------|-------------|
| `backup_id` | string | yes      | The ID of the created backup. |
| `status`    | string | yes      | The status of the backup. |

## Notes

- Backups cannot be updated; recreate the resource to change its name.
- On destroy, the backup is permanently deleted.
