package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// packageIDUpgradePlanModifier warns practitioners that only upgrades
// (not downgrades) are supported by the SHC API when package_id changes.
// Since the current behaviour forces replacement (delete + recreate),
// this modifier also signals RequiresReplace.
type packageIDUpgradePlanModifier struct{}

func (m packageIDUpgradePlanModifier) Description(_ context.Context) string {
	return "When package_id changes, forces replacement and warns that only upgrades are supported in-place by the SHC API."
}

func (m packageIDUpgradePlanModifier) MarkdownDescription(_ context.Context) string {
	return m.Description(context.Background())
}

func (m packageIDUpgradePlanModifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.StateValue.IsNull() || req.PlanValue.IsNull() {
		return
	}

	statePkg := req.StateValue.ValueInt64()
	planPkg := req.PlanValue.ValueInt64()

	if statePkg == planPkg {
		return
	}

	resp.RequiresReplace = true

	if planPkg < statePkg {
		resp.Diagnostics.AddWarning(
			"Package downgrade detected",
			fmt.Sprintf(
				"Changing package_id from %d to %d forces replacement (delete + recreate). "+
					"The SHC API does not support in-place downgrades. The existing VM will be cancelled.",
				statePkg, planPkg,
			),
		)
	} else {
		resp.Diagnostics.AddWarning(
			"Package change forces replacement",
			fmt.Sprintf(
				"Changing package_id from %d to %d forces replacement (delete + recreate). "+
					"Only upgrades are supported in-place by the SHC API, but the provider currently recreates the VM for safety.",
				statePkg, planPkg,
			),
		)
	}
}

func packageIDUpgrade() packageIDUpgradePlanModifier {
	return packageIDUpgradePlanModifier{}
}
