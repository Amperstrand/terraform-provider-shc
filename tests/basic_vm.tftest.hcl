# Test: Create a basic NVMe Starter VM and verify outputs
# Run with: terraform test
# Requires: SHC_API_KEY environment variable

variables {
  hostname = "tftest-basic"
}

run "basic_vm_creation" {
  variables {
    hostname = var.hostname
  }

  module {
    source = "./tests/modules/basic_vm"
  }

  assert {
    condition     = output.vm_ip != null && output.vm_ip != ""
    error_message = "VM should have an IP address"
  }

  assert {
    condition     = output.vm_service_id != null && output.vm_service_id != ""
    error_message = "VM should have a service ID"
  }
}
