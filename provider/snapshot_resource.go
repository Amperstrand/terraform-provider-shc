package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type snapshotResource struct {
	client *SHCClient
}

type snapshotResourceModel struct {
	ServiceID  types.String `tfsdk:"service_id"`
	Name       types.String `tfsdk:"name"`
	SnapshotID types.String `tfsdk:"snapshot_id"`
	Status     types.String `tfsdk:"status"`
}

func NewSnapshotResource() resource.Resource {
	return &snapshotResource{}
}

func (r *snapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshot"
}

func (r *snapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		Description: "Manages a snapshot of an SHC VPS.",
		Attributes: map[string]resourceschema.Attribute{
			"service_id": resourceschema.StringAttribute{
				Required:    true,
				Description: "The SHC service ID of the VPS to snapshot.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": resourceschema.StringAttribute{
				Optional:    true,
				Description: "A name for the snapshot.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"snapshot_id": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The ID of the created snapshot.",
			},
			"status": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The status of the snapshot.",
			},
		},
	}
}

func (r *snapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_snapshot resource")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	r.client = client
}

func (r *snapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan snapshotResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	snapResp, err := r.client.CreateSnapshot(ctx, plan.ServiceID.ValueString(), name)
	if err != nil {
		resp.Diagnostics.AddError("Error creating snapshot", err.Error())
		return
	}

	plan.SnapshotID = types.StringValue(snapResp.ID.String())
	if snapResp.Name != "" {
		plan.Name = types.StringValue(snapResp.Name)
	}
	plan.Status = types.StringValue(snapResp.Status)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *snapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state snapshotResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.SnapshotID.IsNull() || state.SnapshotID.ValueString() == "" {
		return
	}

	snaps, err := r.client.GetSnapshots(ctx, state.ServiceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading snapshots", err.Error())
		return
	}

	found := false
	for _, snap := range snaps {
		if snap.ID.String() == state.SnapshotID.ValueString() {
			found = true
			state.Status = types.StringValue(snap.Status)
			if snap.Name != "" {
				state.Name = types.StringValue(snap.Name)
			}
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *snapshotResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"SHC snapshots cannot be updated. Recreate the resource to change its configuration.",
	)
}

func (r *snapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state snapshotResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSnapshot(ctx, state.ServiceID.ValueString(), state.SnapshotID.ValueString())
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			resp.Diagnostics.AddError("Error deleting snapshot", err.Error())
			return
		}
	}
}

var _ resource.Resource = (*snapshotResource)(nil)
