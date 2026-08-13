package acceptance

import (
	"fmt"
	"os"
)

func ProviderConfig() string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}
`, os.Getenv("SHC_API_KEY"))
}

func VMConfigBasic(hostname string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian13-cloud"
  auto_cancel = true
}
`, ProviderConfig(), hostname)
}

func VMConfigWithSize(hostname, size string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "%s"
  template    = "debian13-cloud"
  auto_cancel = true
}
`, ProviderConfig(), hostname, size)
}

func VMConfigWithTemplate(hostname, template string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "%s"
  auto_cancel = true
}
`, ProviderConfig(), hostname, template)
}

func VMConfigUpdated(hostname string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-2c-8gb"
  template    = "debian13-cloud"
  auto_cancel = true
}
`, ProviderConfig(), hostname)
}

func VMConfigPowerState(hostname, state string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian13-cloud"
  power_state = "%s"
  auto_cancel = true
}
`, ProviderConfig(), hostname, state)
}

func VMConfigInvalidHostname() string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "UPPER CASE!"
  size        = "nvme-1c-4gb"
  auto_cancel = true
}
`, ProviderConfig())
}

func VMConfigInvalidSize() string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "tf-acc-test-vm"
  size        = "nvme-99c-999gb"
  auto_cancel = true
}
`, ProviderConfig())
}

func VMConfigMissingAPIKey() string {
	return `
provider "shc" {
  api_key = ""
}

resource "shc_vm" "test" {
  hostname    = "tf-acc-test-vm"
  size        = "nvme-1c-4gb"
  auto_cancel = true
}
`
}

func SnapshotConfig(hostname, snapshotName string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian13-cloud"
  auto_cancel = true
}

resource "shc_snapshot" "test" {
  service_id = shc_vm.test.service_id
  name       = "%s"
}
`, ProviderConfig(), hostname, snapshotName)
}

func FirewallConfig(hostname string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian13-cloud"
  auto_cancel = true
}

resource "shc_firewall_rule" "ssh" {
  service_id = shc_vm.test.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "22"
  source     = "0.0.0.0/0"
  direction  = "in"
  name       = "allow-ssh"
}
`, ProviderConfig(), hostname)
}

func RDNSConfig(hostname string) string {
	return fmt.Sprintf(`
%s

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian13-cloud"
  auto_cancel = true
}

resource "shc_rdns" "test" {
  service_id = shc_vm.test.service_id
  ip         = shc_vm.test.ip
  hostname   = "%s.example.com"
}
`, ProviderConfig(), hostname, hostname)
}
