package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Default timeouts applied to VM CRUD operations unless overridden by the
// practitioner via the "timeouts" block.
const (
	defaultVMCreateTimeout = 10 * time.Minute
	defaultVMReadTimeout   = 5 * time.Minute
	defaultVMDeleteTimeout = 5 * time.Minute
)

type vmTimeoutsModel struct {
	Create types.String `tfsdk:"create"`
	Read   types.String `tfsdk:"read"`
	Update types.String `tfsdk:"update"`
	Delete types.String `tfsdk:"delete"`
}

type vmResource struct {
	client *SHCClient
}

type vmResourceModel struct {
	Hostname          types.String `tfsdk:"hostname"`
	PackageID         types.Int64  `tfsdk:"package_id"`
	PricingID         types.Int64  `tfsdk:"pricing_id"`
	SSHKey            types.String `tfsdk:"ssh_key"`
	AutoCancel        types.Bool   `tfsdk:"auto_cancel"`
	PowerState        types.String `tfsdk:"power_state"`
	IP                types.String `tfsdk:"ip"`
	ServiceID         types.String `tfsdk:"service_id"`
	OSUser            types.String `tfsdk:"os_user"`
	Status            types.String `tfsdk:"status"`
	ProvisioningState types.String `tfsdk:"provisioning_state"`
	Timeouts          types.Object `tfsdk:"timeouts"`
}

func NewVMResource() resource.Resource {
	return &vmResource{}
}

func (r *vmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (r *vmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		Description: "Manages a Sovereign Hybrid Compute VPS instance.",
		Attributes: map[string]resourceschema.Attribute{
			"hostname": resourceschema.StringAttribute{
				Required:    true,
				Description: "The hostname for the VPS instance.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"package_id": resourceschema.Int64Attribute{
				Required:    true,
				Description: "The SHC package ID (e.g. 81=Standard, 82=Professional, 83=Business).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"pricing_id": resourceschema.Int64Attribute{
				Required:    true,
				Description: "The SHC pricing ID (e.g. 245=Standard, 249=Professional, 253=Business).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"ssh_key": resourceschema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "SSH public key to apply to the VPS after provisioning.",
			},
			"auto_cancel": resourceschema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "If true (default), schedules an end-of-term cancellation so the VPS does not auto-renew.",
				Default:     booldefault.StaticBool(true),
			},
			"power_state": resourceschema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The desired power state: running or stopped. Defaults to running. Changing this triggers a start/stop action without replacing the VM.",
				Default:     stringdefault.StaticString("running"),
			},
			"ip": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The primary IP address of the VPS.",
			},
			"service_id": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The SHC service ID for the VPS.",
			},
			"os_user": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The default OS user for SSH login.",
			},
			"status": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The current service status of the VPS.",
			},
			"provisioning_state": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The provisioning state of the VPS (e.g. ready, provisioning).",
			},
		},
		Blocks: map[string]resourceschema.Block{
			"timeouts": resourceschema.SingleNestedBlock{
				Description: "Customizable timeouts for VM operations. Durations are parsed as Go duration strings (e.g. 10m, 1h).",
				Attributes: map[string]resourceschema.Attribute{
					"create": resourceschema.StringAttribute{
						Optional:    true,
						Description: "Timeout for VM creation. Defaults to 10m.",
					},
					"read": resourceschema.StringAttribute{
						Optional:    true,
						Description: "Timeout for VM read operations. Defaults to 5m.",
					},
					"update": resourceschema.StringAttribute{
						Optional:    true,
						Description: "Timeout for VM update operations.",
					},
					"delete": resourceschema.StringAttribute{
						Optional:    true,
						Description: "Timeout for VM deletion. Defaults to 5m.",
					},
				},
			},
		},
	}
}

func (r *vmResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_vm resource")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	r.client = client
}

func (r *vmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("service_id"), req, resp)
}

func parseTimeoutDuration(ctx context.Context, obj types.Object, key string, def time.Duration) time.Duration {
	if obj.IsNull() || obj.IsUnknown() {
		return def
	}
	var tm vmTimeoutsModel
	if diags := obj.As(ctx, &tm, basetypes.ObjectAsOptions{}); diags.HasError() {
		return def
	}
	var raw types.String
	switch key {
	case "create":
		raw = tm.Create
	case "read":
		raw = tm.Read
	case "delete":
		raw = tm.Delete
	default:
		return def
	}
	if raw.IsNull() || raw.IsUnknown() || raw.ValueString() == "" {
		return def
	}
	d, err := time.ParseDuration(raw.ValueString())
	if err != nil {
		return def
	}
	return d
}

func withTimeout(ctx context.Context, obj types.Object, key string, def time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, parseTimeoutDuration(ctx, obj, key, def))
}

func (r *vmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := withTimeout(ctx, plan.Timeouts, "create", defaultVMCreateTimeout)
	defer cancel()

	orderResp, err := r.client.SubmitOrder(ctx, plan.Hostname.ValueString(), plan.PackageID.ValueInt64(), plan.PricingID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error creating VM", fmt.Sprintf("Could not submit order: %s", err))
		return
	}

	serviceID := orderResp.ResolveServiceID()

	vm, vmDiags := r.waitForProvisioning(ctx, serviceID, resp)
	if vmDiags.HasError() {
		resp.Diagnostics.Append(vmDiags...)
		return
	}

	plan.ServiceID = types.StringValue(serviceID)
	plan.IP = types.StringValue(vm.GetIP())
	plan.Status = types.StringValue(vm.Status)
	plan.ProvisioningState = types.StringValue(vm.ProvisioningState)
	plan.Hostname = types.StringValue(plan.Hostname.ValueString())

	osUser := "debian"
	if vm.OSUser != "" {
		osUser = vm.OSUser
	}
	plan.OSUser = types.StringValue(osUser)

	if !plan.SSHKey.IsNull() && plan.SSHKey.ValueString() != "" {
		if err := r.client.ApplySSHKey(ctx, serviceID, plan.SSHKey.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error applying SSH key", err.Error())
			return
		}
	}

	if plan.AutoCancel.ValueBool() {
		if err := r.client.CancelVM(ctx, serviceID, false); err != nil {
			resp.Diagnostics.AddWarning(
				"Auto-cancel scheduling failed",
				fmt.Sprintf("Could not schedule end-of-term cancellation: %s. The VPS may auto-renew.", err),
			)
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *vmResource) waitForProvisioning(ctx context.Context, serviceID string, resp *resource.CreateResponse) (*VMResponse, diag.Diagnostics) {
	var diags diag.Diagnostics

	const maxAttempts = 120
	const pollInterval = 5 * time.Second

	var lastVM *VMResponse

	for attempt := 0; attempt < maxAttempts; attempt++ {
		vm, err := r.client.GetVM(ctx, serviceID)
		if err != nil && !errors.Is(err, ErrVMNotFound) {
			if attempt < maxAttempts-1 {
				select {
				case <-ctx.Done():
					diags.AddError("Context cancelled", fmt.Sprintf("Context cancelled while waiting for VM %s: %s", serviceID, ctx.Err()))
					return nil, diags
				case <-time.After(pollInterval):
				}
				continue
			}
		}

		if err == nil {
			lastVM = vm
			if vm.ProvisioningState == "ready" {
				return vm, nil
			}
		}

		select {
		case <-ctx.Done():
			diags.AddError("Context cancelled", fmt.Sprintf("Context cancelled while waiting for VM %s to provision: %s", serviceID, ctx.Err()))
			return nil, diags
		case <-time.After(pollInterval):
		}
	}

	if lastVM != nil {
		diags.AddError(
			"VM provisioning timeout",
			fmt.Sprintf("VM %s did not reach 'ready' state after %d attempts. Last state: %s", serviceID, maxAttempts, lastVM.ProvisioningState),
		)
	} else {
		diags.AddError(
			"VM provisioning timeout",
			fmt.Sprintf("VM %s did not reach 'ready' state after %d attempts. VM was not yet available.", serviceID, maxAttempts),
		)
	}
	return nil, diags
}

func (r *vmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ServiceID.IsNull() || state.ServiceID.ValueString() == "" {
		return
	}

	ctx, cancel := withTimeout(ctx, state.Timeouts, "read", defaultVMReadTimeout)
	defer cancel()

	vm, err := r.client.GetVM(ctx, state.ServiceID.ValueString())
	if err != nil {
		if errors.Is(err, ErrVMNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VM", err.Error())
		return
	}

	state.IP = types.StringValue(vm.GetIP())
	state.Status = types.StringValue(vm.Status)
	state.ProvisioningState = types.StringValue(vm.ProvisioningState)
	state.Hostname = types.StringValue(vm.Hostname)
	state.OSUser = types.StringValue(vm.OSUser)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmResourceModel

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

	oldPower := state.PowerState.ValueString()
	newPower := plan.PowerState.ValueString()

	if newPower != oldPower {
		switch newPower {
		case "stopped":
			if err := r.client.SetPowerState(ctx, state.ServiceID.ValueString(), "stop"); err != nil {
				resp.Diagnostics.AddError("Error stopping VM", err.Error())
				return
			}
		case "running":
			if err := r.client.SetPowerState(ctx, state.ServiceID.ValueString(), "start"); err != nil {
				resp.Diagnostics.AddError("Error starting VM", err.Error())
				return
			}
		}
	}

	plan.ServiceID = state.ServiceID
	plan.IP = state.IP
	plan.OSUser = state.OSUser
	plan.Status = state.Status
	plan.ProvisioningState = state.ProvisioningState

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *vmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := withTimeout(ctx, state.Timeouts, "delete", defaultVMDeleteTimeout)
	defer cancel()

	err := r.client.CancelVM(ctx, state.ServiceID.ValueString(), true)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting VM", err.Error())
		return
	}
}

var _ resource.Resource = (*vmResource)(nil)
var _ resource.ResourceWithImportState = (*vmResource)(nil)

type vmDataSource struct {
	client *SHCClient
}

type vmDataSourceModel struct {
	ServiceID         types.String `tfsdk:"service_id"`
	Hostname          types.String `tfsdk:"hostname"`
	IP                types.String `tfsdk:"ip"`
	OSUser            types.String `tfsdk:"os_user"`
	Status            types.String `tfsdk:"status"`
	ProvisioningState types.String `tfsdk:"provisioning_state"`
}

func NewVMDataSource() datasource.DataSource {
	return &vmDataSource{}
}

func (d *vmDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (d *vmDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing SHC VPS by service ID.",
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Required:    true,
				Description: "The SHC service ID of the VPS to read.",
			},
			"hostname": schema.StringAttribute{
				Computed: true,
			},
			"ip": schema.StringAttribute{
				Computed: true,
			},
			"os_user": schema.StringAttribute{
				Computed: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
			"provisioning_state": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *vmDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_vm data source")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	d.client = client
}

func (d *vmDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state vmDataSourceModel
	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ServiceID.IsNull() || state.ServiceID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing service_id", "The service_id attribute is required to read a VM.")
		return
	}

	vm, err := d.client.GetVM(ctx, state.ServiceID.ValueString())
	if err != nil {
		if errors.Is(err, ErrVMNotFound) {
			resp.Diagnostics.AddError("VM not found", fmt.Sprintf("No VM found with service ID %s", state.ServiceID.ValueString()))
			return
		}
		resp.Diagnostics.AddError("Error reading VM", err.Error())
		return
	}

	state.Hostname = types.StringValue(vm.Hostname)
	state.IP = types.StringValue(vm.GetIP())
	state.OSUser = types.StringValue(vm.OSUser)
	state.Status = types.StringValue(vm.Status)
	state.ProvisioningState = types.StringValue(vm.ProvisioningState)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

var _ datasource.DataSource = (*vmDataSource)(nil)
