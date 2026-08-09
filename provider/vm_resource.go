package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)// Default timeouts applied to VM CRUD operations unless overridden by the
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

var _ resource.ResourceWithUpgradeState = (*vmResource)(nil)

func (r *vmResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				resp.State.Set(ctx, req.State.Raw)
			},
		},
	}
}

type vmResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Hostname          types.String `tfsdk:"hostname"`
	Size              types.String `tfsdk:"size"`
	PackageID         types.Int64  `tfsdk:"package_id"`
	PricingID         types.Int64  `tfsdk:"pricing_id"`
	DiskGB            types.Int64  `tfsdk:"disk_gb"`
	RamMB             types.Int64  `tfsdk:"ram_mb"`
	CPU               types.Int64  `tfsdk:"cpu"`
	Template          types.String `tfsdk:"template"`
	SSHKey            types.String `tfsdk:"ssh_key"`
	AutoCancel        types.Bool   `tfsdk:"auto_cancel"`
	PowerState        types.String `tfsdk:"power_state"`
	Term              types.Int64  `tfsdk:"term"`
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
		"id": resourceschema.StringAttribute{
			Computed:    true,
			Description: "The SHC service ID. Equal to service_id.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"hostname": resourceschema.StringAttribute{
			Required:    true,
			Description: "The hostname for the VPS instance.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
			Validators: []validator.String{
				hostname(),
			},
		},
		"package_id": resourceschema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Description: "The SHC package ID. Use `data shc_catalog` to discover valid values, or use `size` for a human-readable alias. Changing this triggers an in-place upgrade; only upgrades (more CPU/RAM/disk) are supported by the SHC API.",
			PlanModifiers: []planmodifier.Int64{
				packageIDUpgrade(),
			},
			Validators: []validator.Int64{
				positiveInt64(),
			},
		},
		"pricing_id": resourceschema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Description: "The SHC pricing ID for the chosen package. Use `data shc_catalog` to discover valid values, or use `size` for a human-readable alias. Changing this triggers an in-place upgrade via the SHC upgrade API.",
			Validators: []validator.Int64{
				positiveInt64(),
			},
		},
		"size": resourceschema.StringAttribute{
			Optional:    true,
			Description: "Spec-encoding size name: {line}-{cpu}c-{ram}gb (e.g. nvme-2c-8gb, hdd-1c-4gb, ssd-4c-16gb, dev-8c-32gb). Takes precedence over package_id/pricing_id when both are set.",
			Validators: []validator.String{
				sizeValidatorFn(),
			},
		},
		"disk_gb": resourceschema.Int64Attribute{
			Optional: true,
			Description: "Override total disk in GB. Resolved to the package's config option at order time. Must be an available value for the selected plan.",
			Validators: []validator.Int64{
				positiveInt64(),
			},
		},
		"ram_mb": resourceschema.Int64Attribute{
			Optional: true,
			Description: "Override total RAM in MB. Resolved to the package's config option at order time.",
			Validators: []validator.Int64{
				positiveInt64(),
			},
		},
		"cpu": resourceschema.Int64Attribute{
			Optional: true,
			Description: "Override total vCPU cores. Resolved to the package's config option at order time.",
			Validators: []validator.Int64{
				positiveInt64(),
			},
		},
		"template": resourceschema.StringAttribute{
			Optional: true,
			Description: "OS template slug (e.g. debian12-cloud, ubuntu2404-cloud). Resolved to the package's config option at order time.",
		},
			"ssh_key": resourceschema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				WriteOnly:   true,
				Description: "SSH public key to apply to the VPS after provisioning. Write-only: not stored in state.",
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
			Description: "The desired power state: `running` or `stopped`. Defaults to `running`. Changing this triggers a start/stop action without replacing the VM.",
			Default:     stringdefault.StaticString("running"),
			Validators: []validator.String{
				powerState(),
			},
		},
	"term": resourceschema.Int64Attribute{
			Optional:    true,
			Description: "Billing term (pricing_id of the desired term, e.g. 56=daily, 57=weekly, 58=monthly). Changing this triggers a term change. Use `shc info <service_id>` or GET /vm/{id}/term-options to see available terms. If unset, the API default (monthly) is used.",
		},
			"ip": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The primary IP address of the VPS.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The SHC service ID for the VPS.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"os_user": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The default OS user for SSH login.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
		Version: 1,
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
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), req.ID)...)
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

	tflog.Info(ctx, "Creating SHC VM", map[string]any{
		"hostname": plan.Hostname.ValueString(),
		"size":     plan.Size.ValueString(),
	})

	// Resolve the size abstraction. If `size` is set it takes precedence over
	// package_id/pricing_id. At least one of (size) or (package_id+pricing_id)
	// must be provided, otherwise we cannot submit an order.
	if !plan.Size.IsNull() && plan.Size.ValueString() != "" {
		pkgID, priceID, err := resolveSize(plan.Size.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid size", err.Error())
			return
		}
		plan.PackageID = types.Int64Value(pkgID)
		plan.PricingID = types.Int64Value(priceID)
	} else {
		if plan.PackageID.ValueInt64() <= 0 || plan.PricingID.ValueInt64() <= 0 {
			resp.Diagnostics.AddError(
				"Missing size or package_id/pricing_id",
				"Provide either 'size' (e.g. \"standard\") or both 'package_id' and 'pricing_id'.",
			)
			return
		}
	}

	ctx, cancel := withTimeout(ctx, plan.Timeouts, "create", defaultVMCreateTimeout)
	defer cancel()

	if err := r.client.CheckCredit(ctx, 0.50); err != nil {
		resp.Diagnostics.AddError("Insufficient credit", err.Error())
		return
	}

	var configOptions map[string]string
	if !plan.DiskGB.IsNull() || !plan.RamMB.IsNull() || !plan.CPU.IsNull() || !plan.Template.IsNull() {
		opts, err := r.client.ResolveAddons(ctx, plan.PackageID.ValueInt64(),
			plan.DiskGB, plan.RamMB, plan.CPU, plan.Template)
		if err != nil {
			resp.Diagnostics.AddError("Config option resolution failed", err.Error())
			return
		}
		configOptions = opts
	}

	creditBefore := r.client.SafeCredit(ctx)
	orderResp, err := r.client.SubmitOrder(ctx, plan.Hostname.ValueString(), plan.PackageID.ValueInt64(), plan.PricingID.ValueInt64(), configOptions)
	if err != nil {
		addSHCError(&resp.Diagnostics, "Creating VM", fmt.Errorf("Could not submit order: %w", err))
		return
	}

	serviceID := orderResp.ResolveServiceID()
	tflog.Info(ctx, "VM ordered", map[string]any{"service_id": serviceID})

	if serviceID != "" {
		sid, _ := strconv.ParseInt(serviceID, 10, 64)
		creditAfter := r.client.SafeCredit(ctx)
		if creditBefore >= 0 && creditAfter >= 0 {
			actualCharge := creditBefore - creditAfter
			r.client.costTracker.TrackOrder(ctx, sid, plan.PackageID.ValueInt64(), &actualCharge)
		} else {
			r.client.costTracker.TrackOrder(ctx, sid, plan.PackageID.ValueInt64(), nil)
		}
	}

	vm, vmDiags := r.waitForProvisioning(ctx, serviceID, resp)
	if vmDiags.HasError() {
		resp.Diagnostics.Append(vmDiags...)
		return
	}

	plan.ServiceID = types.StringValue(serviceID)
	plan.ID = types.StringValue(serviceID)
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
			if vm.Status == "active" && vm.GetIP() != "" {
				tflog.Info(ctx, "VM provisioned", map[string]any{
					"service_id": serviceID, "ip": vm.GetIP(), "attempts": attempt,
				})
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
			fmt.Sprintf("VM %s did not reach active+IP state after %d attempts. Last: status=%s, provisioning_state=%s", serviceID, maxAttempts, lastVM.Status, lastVM.ProvisioningState),
		)
	} else {
		diags.AddError(
			"VM provisioning timeout",
			fmt.Sprintf("VM %s did not reach active+IP state after %d attempts. VM was not yet available.", serviceID, maxAttempts),
		)
	}
	return nil, diags
}

func (r *vmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Reading VM state")
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
		addSHCError(&resp.Diagnostics, "Reading VM", err)
		return
	}

	state.IP = types.StringValue(vm.GetIP())
	state.Status = types.StringValue(vm.Status)
	state.ProvisioningState = types.StringValue(vm.ProvisioningState)
	state.Hostname = types.StringValue(vm.Hostname)
	state.OSUser = types.StringValue(vm.OSUser)
	state.ID = state.ServiceID

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating VM")
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

	// Resolve the effective pricing_id: if `size` is set it takes precedence
	// and may resolve to a new plan, triggering an in-place upgrade.
	var effectivePricingID int64
	if !plan.Size.IsNull() && plan.Size.ValueString() != "" {
		pkgID, priceID, err := resolveSize(plan.Size.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid size", err.Error())
			return
		}
		effectivePricingID = priceID
		plan.PackageID = types.Int64Value(pkgID)
		plan.PricingID = types.Int64Value(priceID)
	} else {
		effectivePricingID = plan.PricingID.ValueInt64()
	}

	// In-place upgrade: pricing_ref in the API equals pricing_id from the
	// catalog. The upgrade is queued async (prorated invoice, resize after payment).
	if effectivePricingID != state.PricingID.ValueInt64() {
		if err := r.client.UpgradeVM(ctx, state.ServiceID.ValueString(), effectivePricingID); err != nil {
			resp.Diagnostics.AddError(
				"Error upgrading VM",
				fmt.Sprintf("Could not upgrade VM %s to pricing_id %d: %s", state.ServiceID.ValueString(), effectivePricingID, err),
			)
			return
		}
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

	// Term change (v2.4.3): if the user changed the term pricing_id,
	// call ChangeVMTerm. This is a confirmed (spends money) action.
	if !plan.Term.IsUnknown() && !state.Term.IsUnknown() &&
		plan.Term.ValueInt64() != state.Term.ValueInt64() &&
		plan.Term.ValueInt64() > 0 {
		termBody, _ := json.Marshal(map[string]interface{}{
			"pricing_ref":     plan.Term.ValueInt64(),
			"idempotency_key": fmt.Sprintf("tf-term-%d", time.Now().UnixNano()),
		})
		if _, err := r.client.ChangeVMTerm(ctx, state.ServiceID.ValueString(), termBody); err != nil {
			resp.Diagnostics.AddError(
				"Error changing VM term",
				fmt.Sprintf("Could not change term to pricing_id %d: %s", plan.Term.ValueInt64(), err),
			)
			return
		}
	}

	plan.ServiceID = state.ServiceID
	plan.ID = state.ServiceID
	plan.IP = state.IP
	plan.OSUser = state.OSUser
	plan.Status = state.Status
	plan.ProvisioningState = state.ProvisioningState

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *vmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "Destroying VM")
	var state vmResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := withTimeout(ctx, state.Timeouts, "delete", defaultVMDeleteTimeout)
	defer cancel()

	creditBefore := r.client.SafeCredit(ctx)
	err := r.client.CancelVM(ctx, state.ServiceID.ValueString(), true)
	if err != nil {
		addSHCError(&resp.Diagnostics, "Destroying VM", err)
		return
	}

	sid, _ := strconv.ParseInt(state.ServiceID.ValueString(), 10, 64)
	var actualRefund *float64
	creditAfter := r.client.SafeCredit(ctx)
	if creditBefore >= 0 && creditAfter >= 0 {
		diff := creditAfter - creditBefore
		actualRefund = &diff
	}
	r.client.costTracker.AuditCancel(ctx, sid, actualRefund)
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
		addSHCError(&resp.Diagnostics, "Reading VM", err)
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
