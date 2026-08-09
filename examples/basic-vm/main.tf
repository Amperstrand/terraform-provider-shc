# Minimal SHC VM — one NVMe Starter VPS
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
  hostname    = "web-server"
  size        = "nvme-1c-4gb"
  template    = "debian12-cloud"
  auto_cancel = true
}

output "vm_ip" {
  value = shc_vm.web.ip
}

output "vm_service_id" {
  value = shc_vm.web.service_id
}
