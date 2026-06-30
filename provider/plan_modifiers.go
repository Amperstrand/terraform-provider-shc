package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// packageIDUpgradePlanModifier warns practitioners that only upgrades
// (not downgrades) are supported by the SHC API when package_id changes.
// Upgrades are handled in-place via the Update method; disk-reducing
// downgrades will be rejected by the API with a 422 error.
type packageIDUpgradePlanModifier struct{}

func (m packageIDUpgradePlanModifier) Description(_ context.Context) string {
	return "When package_id changes, triggers an in-place upgrade. Only upgrades are supported by the SHC API; downgrades are rejected."
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

	if planPkg < statePkg {
		resp.Diagnostics.AddWarning(
			"Package downgrade detected",
			fmt.Sprintf(
				"Changing package_id from %d to %d will trigger an in-place upgrade request, "+
					"but the SHC API does not support downgrades (disk-reducing changes are rejected with a 422 error). "+
					"The apply will fail unless the target package has equal or greater resources.",
				statePkg, planPkg,
			),
		)
	}
}

func packageIDUpgrade() packageIDUpgradePlanModifier {
	return packageIDUpgradePlanModifier{}
}
