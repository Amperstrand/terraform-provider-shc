package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfig("tf-acc-basic"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
					resource.TestCheckResourceAttr("shc_vm.test", "provisioning_state", "ready"),
				),
			},
		},
	})
}

func TestAccVMResource_WithSize(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfigWithSize("tf-acc-size", "dev-1c-4gb"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
					resource.TestCheckResourceAttr("shc_vm.test", "provisioning_state", "ready"),
				),
			},
		},
	})
}

func TestAccVMResource_WithTemplate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfigWithTemplate("tf-acc-tmpl", "debian12-cloud"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
					resource.TestCheckResourceAttrSet("shc_vm.test", "ip"),
					resource.TestCheckResourceAttr("shc_vm.test", "provisioning_state", "ready"),
				),
			},
		},
	})
}

func TestAccVMResource_Import(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccVMResourceConfig("tf-acc-import"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
				),
			},
			{
				ResourceName:      "shc_vm.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"auto_cancel", "ssh_key", "timeouts",
				},
			},
		},
	})
}

func testAccVMResourceConfig(hostname string) string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

resource "shc_vm" "test" {
  hostname    = "%s"
  package_id  = 81
  pricing_id  = 245
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
  size        = "dev-1c-4gb"
  template    = "%s"
  auto_cancel = true
}
`, os.Getenv("SHC_API_KEY"), hostname, template)
}
