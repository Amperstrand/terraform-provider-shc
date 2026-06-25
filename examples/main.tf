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
  hostname    = "tf-test"
  package_id  = 81
  pricing_id  = 245
  auto_cancel = true
}

resource "shc_snapshot" "test" {
  service_id = shc_vm.test.service_id
  name       = "tf-test-snapshot"
}

output "vm_ip" {
  value = shc_vm.test.ip
}

output "vm_service_id" {
  value = shc_vm.test.service_id
}

output "snapshot_id" {
  value = shc_snapshot.test.snapshot_id
}
