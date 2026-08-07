package acceptance

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func CheckVMDestroy(s *terraform.State) error {
	client := TestClient()
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
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("unexpected error checking VM %s: %s", serviceID, err)
		}
	}
	return nil
}

func CheckSnapshotDestroy(s *terraform.State) error {
	client := TestClient()
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

func CheckFirewallDestroy(s *terraform.State) error {
	client := TestClient()
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
