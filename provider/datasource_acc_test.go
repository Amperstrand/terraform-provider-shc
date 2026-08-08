package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSource_Catalog(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: testAccCatalogDataSourceConfig(),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.shc_catalog.current", "packages.#"),
			),
		}},
	})
}

func TestAccDataSource_Templates(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: testAccTemplatesDataSourceConfig(),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.shc_templates.available", "templates.#"),
			),
		}},
	})
}

func TestAccDataSource_VM(t *testing.T) {
	hostname := "tf-acc-test-ds-" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{{
			Config: testAccVMDataSourceConfig(hostname),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrSet("shc_vm.test", "service_id"),
				resource.TestCheckResourceAttrSet("data.shc_vm.existing", "hostname"),
				resource.TestCheckResourceAttr("data.shc_vm.existing", "hostname", hostname),
			),
		}},
	})
}

func testAccCatalogDataSourceConfig() string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

data "shc_catalog" "current" {}
`, os.Getenv("SHC_API_KEY"))
}

func testAccTemplatesDataSourceConfig() string {
	return fmt.Sprintf(`
provider "shc" {
  api_key = "%s"
}

data "shc_templates" "available" {}
`, os.Getenv("SHC_API_KEY"))
}

func testAccVMDataSourceConfig(hostname string) string {
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

data "shc_vm" "existing" {
  service_id = shc_vm.test.service_id
}
`, os.Getenv("SHC_API_KEY"), hostname)
}
