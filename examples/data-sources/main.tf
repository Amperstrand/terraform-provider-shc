# Using data sources for inventory and cost analysis
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

# List all VMs on the account
data "shc_vms" "all" {}

# Check billing
data "shc_billing" "current" {}

# Browse available plans
data "shc_catalog" "catalog" {}

# Cost estimation
output "nvme_standard_monthly" {
  value = provider::shc::estimate_cost("nvme-2c-8gb", 30, "days").total_cost
}

output "vm_count" {
  value = length(data.shc_vms.all.vms)
}

output "vm_hostnames" {
  value = [for vm in data.shc_vms.all.vms : "${vm.hostname} (${vm.status})"]
}

output "credit" {
  value = data.shc_billing.current.credit
}
