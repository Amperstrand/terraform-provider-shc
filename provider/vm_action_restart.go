package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// vmRestartAction (#32): a Terraform 1.14+ action — invoke VM restart
// without modeling it as an Update on the managed resource. Restart is a
// routine (ungated) power op in SHC's operator contract, so Invoke is a
// single POST with no confirmation flow.
var _ action.ActionWithConfigure = (*vmRestartAction)(nil)

type vmRestartAction struct {
	client *SHCClient
}

// NewVMRestartAction returns a fresh shc_vm_restart action.
func NewVMRestartAction() action.Action {
	return &vmRestartAction{}
}

func (a *vmRestartAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_restart"
}

func (a *vmRestartAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Restarts a VM. Invoked via `terraform apply` with an action block; no state is created or mutated.",
		Attributes: map[string]schema.Attribute{
			"vm_id": schema.StringAttribute{
				Required:    true,
				Description: "Service id of the VM to restart.",
			},
		},
	}
}

func (a *vmRestartAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*SHCClient)
	if !ok {
		resp.Diagnostics.AddError("unexpected provider data", "expected *SHCClient")
		return
	}
	a.client = client
}

func (a *vmRestartAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config struct {
		VMID types.String `tfsdk:"vm_id"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError("provider not configured", "missing SHC client")
		return
	}
	if err := a.client.SetPowerState(ctx, config.VMID.ValueString(), "restart"); err != nil {
		resp.Diagnostics.AddError("restarting VM", fmt.Sprintf("vm %s: %v", config.VMID.ValueString(), err))
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("restart issued for VM %s", config.VMID.ValueString()),
		})
	}
}
