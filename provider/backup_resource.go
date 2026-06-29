package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type backupResource struct {
	client *SHCClient
}

type backupResourceModel struct {
	ServiceID types.String `tfsdk:"service_id"`
	Name      types.String `tfsdk:"name"`
	BackupID  types.String `tfsdk:"backup_id"`
	Status    types.String `tfsdk:"status"`
}

func NewBackupResource() resource.Resource {
	return &backupResource{}
}

func (r *backupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup"
}

func (r *backupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		Description: "Manages a backup of an SHC VPS.",
		Attributes: map[string]resourceschema.Attribute{
			"service_id": resourceschema.StringAttribute{
				Required:    true,
				Description: "The SHC service ID of the VPS to back up.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": resourceschema.StringAttribute{
				Optional:    true,
				Description: "A name for the backup.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"backup_id": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The ID of the created backup.",
			},
			"status": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The status of the backup.",
			},
		},
	}
}

func (r *backupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_backup resource")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	r.client = client
}

func (r *backupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the format service_id:backup_id.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("backup_id"), parts[1])...)
}

func (r *backupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan backupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	backupResp, err := r.client.CreateBackup(ctx, plan.ServiceID.ValueString(), name)
	if err != nil {
		if isStorageUnsupportedErr(err) {
			resp.Diagnostics.AddError(storageUnsupportedSummary, storageUnsupportedDetail)
			return
		}
		resp.Diagnostics.AddError("Error creating backup", err.Error())
		return
	}

	plan.BackupID = types.StringValue(backupResp.ID.String())
	if backupResp.Name != "" {
		plan.Name = types.StringValue(backupResp.Name)
	}
	plan.Status = types.StringValue(backupResp.Status)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *backupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state backupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.BackupID.IsNull() || state.BackupID.ValueString() == "" {
		return
	}

	backups, err := r.client.GetBackups(ctx, state.ServiceID.ValueString())
	if err != nil {
		if isStorageUnsupportedErr(err) {
			resp.Diagnostics.AddError(storageUnsupportedSummary, storageUnsupportedDetail)
			return
		}
		resp.Diagnostics.AddError("Error reading backups", err.Error())
		return
	}

	found := false
	for _, backup := range backups {
		if backup.ID.String() == state.BackupID.ValueString() {
			found = true
			state.Status = types.StringValue(backup.Status)
			if backup.Name != "" {
				state.Name = types.StringValue(backup.Name)
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

func (r *backupResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"SHC backups cannot be updated. Recreate the resource to change its configuration.",
	)
}

func (r *backupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state backupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteBackup(ctx, state.ServiceID.ValueString(), state.BackupID.ValueString())
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			resp.Diagnostics.AddError("Error deleting backup", err.Error())
			return
		}
	}
}

var _ resource.Resource = (*backupResource)(nil)
var _ resource.ResourceWithImportState = (*backupResource)(nil)
