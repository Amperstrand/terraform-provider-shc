package acceptance

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func init() {
	resource.AddTestSweepers("shc_vm", &resource.Sweeper{
		Name: "shc_vm",
		F:    sweepVMs,
	})
}

func sweepVMs(_ string) error {
	client := TestClient()

	listResp, err := client.ListVMsForSweep()
	if err != nil {
		return fmt.Errorf("listing VMs for sweep: %w", err)
	}

	var swept int
	for _, vm := range listResp {
		if strings.HasPrefix(vm.Hostname, TestNamePrefix) {
			log.Printf("[DEBUG] Sweeping SHC VM %s (hostname=%s)", vm.ServiceID, vm.Hostname)
			if err := client.CancelVM(context.Background(), vm.ServiceID, true); err != nil {
				log.Printf("[WARN] Error sweeping VM %s: %s", vm.ServiceID, err)
			}
			swept++
		}
	}

	log.Printf("[DEBUG] Swept %d SHC VMs", swept)
	return nil
}
