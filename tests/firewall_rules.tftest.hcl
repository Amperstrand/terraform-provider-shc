# Test: Create VM with firewall rules and verify they work
# Run with: terraform test

variables {
  hostname = "tftest-firewall"
}

run "vm_with_firewall" {
  variables {
    hostname = var.hostname
  }

  module {
    source = "./tests/modules/vm_firewall"
  }

  assert {
    condition     = output.vm_ip != null && output.vm_ip != ""
    error_message = "VM should have an IP"
  }

  assert {
    condition     = length(shc_firewall_rule.ssh.id) > 0
    error_message = "Firewall rule should be created"
  }
}
