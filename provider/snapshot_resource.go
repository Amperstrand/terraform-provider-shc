package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const storageUnsupportedSummary = "Storage feature not available for this VPS plan"

const storageUnsupportedDetail = "This VPS plan may not support snapshots/backups. Dev VPS plans (pkg 80-84) do not have storage features. Use NVMe/SSD/HDD VPS plans instead."

func isStorageUnsupportedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "upstream_failure") || strings.Contains(msg, "unable to load storage inventory")
}

type snapshotResource struct {
	client *SHCClient
}

type snapshotResourceModel struct {
	ServiceID  types.String `tfsdk:"service_id"`
	Name       types.String `tfsdk:"name"`
	Restore    types.Bool   `tfsdk:"restore"`
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
			"restore": resourceschema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "When true, triggers a one-shot restore of the VM from this snapshot on the next apply. Resets to false after the restore completes.",
				Default:     booldefault.StaticBool(false),
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

func (r *snapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the format service_id:snapshot_id.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("snapshot_id"), parts[1])...)
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
		if isStorageUnsupportedErr(err) {
			resp.Diagnostics.AddError(storageUnsupportedSummary, storageUnsupportedDetail)
			return
		}
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
		if isStorageUnsupportedErr(err) {
			resp.Diagnostics.AddError(storageUnsupportedSummary, storageUnsupportedDetail)
			return
		}
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

func (r *snapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state snapshotResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// One-shot restore action: triggered when restore changes from false to true.
	if plan.Restore.ValueBool() && !state.Restore.ValueBool() {
		err := r.client.RestoreSnapshot(ctx, state.ServiceID.ValueString(), state.SnapshotID.ValueString())
		if err != nil {
			if isStorageUnsupportedErr(err) {
				resp.Diagnostics.AddError(storageUnsupportedSummary, storageUnsupportedDetail)
				return
			}
			resp.Diagnostics.AddError("Error restoring snapshot", err.Error())
			return
		}
	}

	// Restore is a one-shot action: always reset to false after processing.
	plan.Restore = types.BoolValue(false)

	// Carry forward computed fields from state.
	plan.SnapshotID = state.SnapshotID
	plan.Status = state.Status

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
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
var _ resource.ResourceWithImportState = (*snapshotResource)(nil)
