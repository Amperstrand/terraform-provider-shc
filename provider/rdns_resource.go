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

type rdnsResource struct {
	client *SHCClient
}

type rdnsResourceModel struct {
	ServiceID types.String `tfsdk:"service_id"`
	IP        types.String `tfsdk:"ip"`
	Hostname  types.String `tfsdk:"hostname"`
	JobID     types.String `tfsdk:"job_id"`
}

func NewRDNSResource() resource.Resource {
	return &rdnsResource{}
}

func (r *rdnsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rdns"
}

func (r *rdnsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		Description: "Manages reverse DNS (PTR record) for an IP address on an SHC VPS instance.",
		Attributes: map[string]resourceschema.Attribute{
			"service_id": resourceschema.StringAttribute{
				Required:    true,
				Description: "The SHC service ID of the VPS.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip": resourceschema.StringAttribute{
				Required:    true,
				Description: "The IP address to set reverse DNS for.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hostname": resourceschema.StringAttribute{
				Required:    true,
				Description: "The FQDN to set as the PTR record.",
			},
			"job_id": resourceschema.StringAttribute{
				Computed:    true,
				Description: "The async job ID for the rDNS operation.",
			},
		},
	}
}

func (r *rdnsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_rdns resource")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	r.client = client
}

func (r *rdnsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the format service_id:ip.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), parts[1])...)
}

func (r *rdnsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan rdnsResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rdnsResp, err := r.client.SetReverseDNS(ctx, plan.ServiceID.ValueString(), plan.IP.ValueString(), plan.Hostname.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating rDNS record", err.Error())
		return
	}

	plan.JobID = types.StringValue(rdnsResp.JobID.String())

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *rdnsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state rdnsResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.IP.IsNull() || state.IP.ValueString() == "" {
		return
	}

	records, err := r.client.GetReverseDNS(ctx, state.ServiceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading rDNS", err.Error())
		return
	}

	found := false
	targetIP := state.IP.ValueString()
	for _, rec := range records {
		if rec.IP == targetIP {
			found = true
			state.Hostname = types.StringValue(rec.Hostname)
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

func (r *rdnsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state rdnsResourceModel
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

	// Only hostname can change; service_id and ip force replacement.
	if plan.Hostname.ValueString() != state.Hostname.ValueString() {
		rdnsResp, err := r.client.SetReverseDNS(ctx, plan.ServiceID.ValueString(), plan.IP.ValueString(), plan.Hostname.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating rDNS record", err.Error())
			return
		}
		plan.JobID = types.StringValue(rdnsResp.JobID.String())
	} else {
		plan.JobID = state.JobID
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *rdnsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state rdnsResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.ClearReverseDNS(ctx, state.ServiceID.ValueString(), state.IP.ValueString())
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			resp.Diagnostics.AddError("Error deleting rDNS record", err.Error())
			return
		}
	}
}

var _ resource.Resource = (*rdnsResource)(nil)
var _ resource.ResourceWithImportState = (*rdnsResource)(nil)
