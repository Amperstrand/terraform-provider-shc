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
  description = "Hostname for the CI runner"
  default     = "ci-runner"
}

variable "package_id" {
  type        = number
  description = "SHC package ID (26 = NVMe Standard, 2C/8GB/16GB)"
  default     = 26
}

variable "pricing_id" {
  type        = number
  description = "SHC pricing ID for the package"
  default     = 55
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

variable "power_state" {
  type        = string
  description = "Desired VM power state: running or stopped"
  default     = "running"
}

provider "shc" {
  api_key = var.shc_api_key
}

resource "shc_vm" "ci_runner" {
  hostname    = var.hostname
  package_id  = var.package_id
  pricing_id  = var.pricing_id
  ssh_key     = file(var.ssh_public_key)
  auto_cancel = true
  power_state = var.power_state
}

resource "shc_firewall_rule" "allow_ssh" {
  service_id = shc_vm.ci_runner.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "22"
  source     = join(",", var.ssh_source_ranges)
  name       = "allow-ssh"
}

output "vm_ip" {
  description = "Public IP address of the CI runner"
  value       = shc_vm.ci_runner.ip
}

output "service_id" {
  description = "SHC service ID for the VM"
  value       = shc_vm.ci_runner.service_id
}

output "ssh_command" {
  description = "SSH command to connect to the CI runner"
  value       = "ssh debian@${shc_vm.ci_runner.ip}"
}