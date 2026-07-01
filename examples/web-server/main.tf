terraform {
  required_providers {
    shc = {
      source = "sovereignhybridcompute/shc"
    }
  }
}

variable "shc_api_key" {
  type        = string
  description = "SHC API key for authentication"
  sensitive   = true
}

variable "hostname" {
  type        = string
  description = "Hostname for the web server"
  default     = "web-server"
}

variable "ssh_public_key" {
  type        = string
  description = "Path to SSH public key file"
  default     = "~/.ssh/id_rsa.pub"
}

variable "ssh_source_ranges" {
  type        = list(string)
  description = "Allowed CIDR ranges for SSH access"
  default     = ["0.0.0.0/0"]
}

variable "snapshot_name" {
  type        = string
  description = "Name for the initial snapshot"
  default     = "initial-deploy"
}

provider "shc" {
  api_key = var.shc_api_key
}

resource "shc_vm" "web_server" {
  hostname    = var.hostname
  size        = "standard"
  ssh_key     = file(var.ssh_public_key)
  auto_cancel = true
  power_state = "running"
  nodns       = true
  nodns_zone  = "dns4sats.xyz"
}

resource "shc_firewall_rule" "allow_ssh" {
  service_id = shc_vm.web_server.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "22"
  name       = "allow-ssh"
}

resource "shc_firewall_rule" "allow_http" {
  service_id = shc_vm.web_server.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "80"
  source     = "0.0.0.0/0"
  name       = "allow-http"
}

resource "shc_firewall_rule" "allow_https" {
  service_id = shc_vm.web_server.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "443"
  source     = "0.0.0.0/0"
  name       = "allow-https"
}

resource "shc_snapshot" "initial_deploy" {
  service_id = shc_vm.web_server.service_id
  name       = var.snapshot_name
}

output "vm_ip" {
  description = "Public IP address of the web server"
  value       = shc_vm.web_server.ip
}

output "vm_fqdn" {
  description = "NoDNS FQDN pointing to the web server"
  value       = shc_vm.web_server.fqdn
}

output "service_id" {
  description = "SHC service ID for the VM"
  value       = shc_vm.web_server.service_id
}

output "snapshot_id" {
  description = "ID of the initial snapshot"
  value       = shc_snapshot.initial_deploy.snapshot_id
}

output "ssh_command" {
  description = "SSH command to connect to the web server"
  value       = "ssh debian@${shc_vm.web_server.ip}"
}