# VM with SSH + HTTPS firewall rules
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

resource "shc_vm" "app" {
  hostname    = "app-server"
  size        = "nvme-2c-8gb"
  template    = "ubuntu2404-cloud"
  ssh_key     = file("~/.ssh/id_ed25519.pub")
  auto_cancel = true
}

resource "shc_firewall_rule" "ssh" {
  service_id = shc_vm.app.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "22"
  source     = "0.0.0.0/0"
  direction  = "in"
  name       = "allow-ssh"
}

resource "shc_firewall_rule" "https" {
  service_id = shc_vm.app.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "443"
  source     = "0.0.0.0/0"
  direction  = "in"
  name       = "allow-https"
}

resource "shc_snapshot" "pre_deploy" {
  service_id = shc_vm.app.service_id
  name       = "pre-deploy"
}

output "vm_ip" {
  value = shc_vm.app.ip
}
