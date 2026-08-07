package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccPreCheck(t *testing.T) {
	if v := os.Getenv("SHC_API_KEY"); v == "" {
		t.Fatal("SHC_API_KEY must be set for acceptance tests")
	}
}

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"shc": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func TestAccVMResource_Basic(t *testing.T) {
	hostname := "tf-acc-test-vm-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfig(hostname),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
					resource.TestCheckResourceAttr("shc_vm.test", "status", "active"),
					resource.TestCheckResourceAttr("shc_vm.test", "hostname", hostname),
				),
			},
		},
	})
}

func TestAccVMResource_UpdateSize(t *testing.T) {
	hostname := "tf-acc-test-vm-upd-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfigWithSize(hostname, "nvme-1c-4gb"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
					resource.TestCheckResourceAttr("shc_vm.test", "status", "active"),
				),
			},
			{
				Config: testAccVMResourceConfigWithSize(hostname, "nvme-2c-8gb"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
				),
			},
		},
	})
}

func TestAccVMResource_WithTemplate(t *testing.T) {
	hostname := "tf-acc-test-vm-tmpl-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfigWithTemplate(hostname, "debian12-cloud"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
					resource.TestCheckResourceAttr("shc_vm.test", "status", "active"),
				),
			},
		},
	})
}

func TestAccVMResource_Import(t *testing.T) {
	hostname := "tf-acc-test-vm-imp-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfig(hostname),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
				),
			},
			{
				ResourceName:            "shc_vm.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{
					"auto_cancel", "ssh_key", "timeouts",
					"size", "template", "package_id", "pricing_id", "power_state", "term",
				},
			},
		},
	})
}

func TestAccVMResource_InvalidHostname(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      testAccVMResourceInvalidHostname(),
				ExpectError: regexp.MustCompile(`(?i)invalid hostname`),
			},
		},
	})
}

func TestAccVMResource_InvalidSize(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      testAccVMResourceInvalidSize(),
				ExpectError: regexp.MustCompile(`(?i)invalid size`),
			},
		},
	})
}

func testAccCheckVMDestroy(s *terraform.State) error {
	client := NewSHCClient(os.Getenv("SHC_API_KEY"), "")
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "shc_vm" {
			continue
		}
		serviceID := rs.Primary.Attributes["service_id"]
		if serviceID == "" {
			continue
		}
		_, err := client.GetVM(context.Background(), serviceID)
		if err == nil {
			return fmt.Errorf("SHC VM %s still exists", serviceID)
		}
	}
	return nil
}

func testAccVMResourceConfig(hostname string) string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "debian12-cloud"
  auto_cancel = true
}
`, os.Getenv("SHC_API_KEY"), hostname)
}

func testAccVMResourceConfigWithSize(hostname, size string) string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "%s"
  template    = "debian12-cloud"
  auto_cancel = true
}
`, os.Getenv("SHC_API_KEY"), hostname, size)
}

func testAccVMResourceConfigWithTemplate(hostname, template string) string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

resource "shc_vm" "test" {
  hostname    = "%s"
  size        = "nvme-1c-4gb"
  template    = "%s"
  auto_cancel = true
}
`, os.Getenv("SHC_API_KEY"), hostname, template)
}

func testAccVMResourceInvalidHostname() string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

resource "shc_vm" "test" {
  hostname    = "UPPER CASE!"
  size        = "nvme-1c-4gb"
  auto_cancel = true
}
`, os.Getenv("SHC_API_KEY"))
}

func testAccVMResourceInvalidSize() string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

resource "shc_vm" "test" {
  hostname    = "tf-acc-test-vm"
  size        = "nvme-99c-999gb"
  auto_cancel = true
}
`, os.Getenv("SHC_API_KEY"))
}
