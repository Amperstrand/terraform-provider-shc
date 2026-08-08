package provider

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccSnapshotResource_Basic(t *testing.T) {
	hostname := "tf-acc-test-snap-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckSnapshotDestroy,
		Steps: []resource.TestStep{{
			Config: testAccSnapshotConfig(hostname),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
				resource.TestCheckResourceAttrSet("shc_snapshot.test", "snapshot_id"),
				resource.TestCheckResourceAttr("shc_snapshot.test", "name", "pre-deploy"),
			),
		}},
	})
}

func TestAccFirewallRuleResource_Basic(t *testing.T) {
	hostname := "tf-acc-test-fw-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckFirewallDestroy,
		Steps: []resource.TestStep{{
			Config: testAccFirewallConfig(hostname),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
				resource.TestCheckResourceAttrSet("shc_firewall_rule.ssh", "position"),
				resource.TestCheckResourceAttr("shc_firewall_rule.ssh", "protocol", "tcp"),
				resource.TestCheckResourceAttr("shc_firewall_rule.ssh", "port", "22"),
			),
		}},
	})
}

func TestAccRDNSResource_Basic(t *testing.T) {
	t.Skip("RDNS requires FCrDNS — hostname must resolve to VM IP. Cannot satisfy in test env without real DNS.")
	hostname := "tf-acc-test-rdns-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{{
			Config: testAccRDNSConfig(hostname),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
				resource.TestCheckResourceAttrSet("shc_rdns.test", "job_id"),
			),
		}},
	})
}

func testAccCheckSnapshotDestroy(s *terraform.State) error {
	client := NewSHCClient(os.Getenv("SHC_API_KEY"), "")
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "shc_snapshot" {
			continue
		}
		serviceID := rs.Primary.Attributes["service_id"]
		if serviceID == "" {
			continue
		}
		snapshots, err := client.GetSnapshots(context.Background(), serviceID)
		if err == nil && len(snapshots) > 0 {
			return fmt.Errorf("snapshots still exist for VM %s", serviceID)
		}
	}
	return nil
}

func testAccCheckFirewallDestroy(s *terraform.State) error {
	client := NewSHCClient(os.Getenv("SHC_API_KEY"), "")
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "shc_firewall_rule" {
			continue
		}
		serviceID := rs.Primary.Attributes["service_id"]
		if serviceID == "" {
			continue
		}
		fw, err := client.GetFirewall(context.Background(), serviceID)
		if err == nil && fw != nil && len(fw.Rules) > 0 {
			return fmt.Errorf("firewall rules still exist for VM %s", serviceID)
		}
	}
	return nil
}

func testAccSnapshotConfig(hostname string) string {
	return fmt.Sprintf(`
provider "shc" { api_key = "%s" }

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian12-cloud"
  auto_cancel = true
}

resource "shc_snapshot" "test" {
  service_id = shc_vm.test.service_id
  name       = "pre-deploy"
}
`, os.Getenv("SHC_API_KEY"), hostname)
}

func testAccFirewallConfig(hostname string) string {
	return fmt.Sprintf(`
provider "shc" { api_key = "%s" }

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian12-cloud"
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
`, os.Getenv("SHC_API_KEY"), hostname)
}

func testAccRDNSConfig(hostname string) string {
	return fmt.Sprintf(`
provider "shc" { api_key = "%s" }

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian12-cloud"
  auto_cancel = true
}

resource "shc_rdns" "test" {
  service_id = shc_vm.test.service_id
  ip         = shc_vm.test.ip
  hostname   = "%s.example.com"
}
`, os.Getenv("SHC_API_KEY"), hostname, hostname)
}

func testAccFirewallBlockConfig(hostname string) string {
	return fmt.Sprintf(`
provider "shc" { api_key = "%s" }

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian12-cloud"
  auto_cancel = true
}

resource "shc_firewall_rule" "block_ssh" {
  service_id = shc_vm.test.service_id
  action     = "drop"
  protocol   = "tcp"
  port       = "22"
  source     = "0.0.0.0/0"
  direction  = "in"
  name       = "block-ssh"
}
`, os.Getenv("SHC_API_KEY"), hostname)
}

func testAccCheckPortReachable(resourceName, port string, expectOpen bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		ip := rs.Primary.Attributes["ip"]
		if ip == "" {
			return fmt.Errorf("no IP on %s", resourceName)
		}
		addr := net.JoinHostPort(ip, port)
		for attempt := 0; attempt < 12; attempt++ {
			conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
			if expectOpen && err == nil {
				conn.Close()
				return nil
			}
			if !expectOpen && err != nil {
				return nil
			}
			if conn != nil {
				conn.Close()
			}
			time.Sleep(10 * time.Second)
		}
		if expectOpen {
			return fmt.Errorf("port %s on %s not reachable after 2 min", port, ip)
		}
		return fmt.Errorf("port %s on %s unexpectedly open", port, ip)
	}
}

func TestAccFirewallRule_PortAccessible(t *testing.T) {
	hostname := "tf-acc-test-fwport-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{{
			Config: testAccFirewallConfig(hostname),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
				resource.TestCheckResourceAttrSet("shc_firewall_rule.ssh", "position"),
				testAccCheckPortReachable("shc_vm.test", "22", true),
			),
		}},
	})
}

func TestAccFirewallRule_BlockedPort(t *testing.T) {
	hostname := "tf-acc-test-fwblock-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{{
			Config: testAccFirewallBlockConfig(hostname),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
				testAccCheckPortReachable("shc_vm.test", "22", false),
			),
		}},
	})
}

func TestAccVM_Disappears(t *testing.T) {
	hostname := "tf-acc-test-vm-dis-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfig(hostname),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					testAccDeleteVMExternally("shc_vm.test"),
				),
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccVMResourceConfig(hostname),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
				),
			},
		},
	})
}

func testAccDeleteVMExternally(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		serviceID := rs.Primary.Attributes["service_id"]
		client := NewSHCClient(os.Getenv("SHC_API_KEY"), "")
		return client.CancelVM(context.Background(), serviceID, true)
	}
}
