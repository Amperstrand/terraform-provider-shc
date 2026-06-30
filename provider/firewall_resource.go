package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type firewallRuleResource struct {
	client *SHCClient
}

type firewallRuleResourceModel struct {
	ServiceID types.String `tfsdk:"service_id"`
	Action    types.String `tfsdk:"action"`
	Protocol  types.String `tfsdk:"protocol"`
	Port      types.String `tfsdk:"port"`
	Source    types.String `tfsdk:"source"`
	Direction types.String `tfsdk:"direction"`
	Name      types.String `tfsdk:"name"`
	Position  types.Int64  `tfsdk:"position"`
}

func NewFirewallRuleResource() resource.Resource {
	return &firewallRuleResource{}
}

func (r *firewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rule"
}

func (r *firewallRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		Description: "Manages a firewall rule on an SHC VPS instance.",
		Attributes: map[string]resourceschema.Attribute{
			"service_id": resourceschema.StringAttribute{
				Required:    true,
				Description: "The SHC service ID of the VPS.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"action": resourceschema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The firewall action: accept, drop, or reject. Defaults to accept.",
				Default:     stringdefault.StaticString("accept"),
			},
			"protocol": resourceschema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The protocol: tcp, udp, or icmp. Defaults to tcp.",
				Default:     stringdefault.StaticString("tcp"),
			},
			"port": resourceschema.StringAttribute{
				Optional:    true,
				Description: "The destination port (e.g. 22, 80,443).",
			},
			"source": resourceschema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The source CIDR. Defaults to 0.0.0.0/0.",
				Default:     stringdefault.StaticString("0.0.0.0/0"),
			},
			"direction": resourceschema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The direction: in or out. Defaults to in.",
				Default:     stringdefault.StaticString("in"),
			},
			"name": resourceschema.StringAttribute{
				Optional:    true,
				Description: "A label or comment for the rule.",
			},
			"position": resourceschema.Int64Attribute{
				Computed:    true,
				Description: "The position of the rule in the chain.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *firewallRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_firewall_rule resource")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	r.client = client
}

func (r *firewallRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the format service_id:position.",
		)
		return
	}

	var position int64
	if _, err := fmt.Sscanf(parts[1], "%d", &position); err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected position to be an integer, got %q.", parts[1]),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("position"), position)...)
}

func (r *firewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallRuleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ruleBody := map[string]string{
		"action":    plan.Action.ValueString(),
		"protocol":  plan.Protocol.ValueString(),
		"source":    plan.Source.ValueString(),
		"direction": plan.Direction.ValueString(),
	}
	if !plan.Port.IsNull() && plan.Port.ValueString() != "" {
		ruleBody["port"] = plan.Port.ValueString()
	}
	if !plan.Name.IsNull() && plan.Name.ValueString() != "" {
		ruleBody["name"] = plan.Name.ValueString()
	}

	body, err := json.Marshal(ruleBody)
	if err != nil {
		resp.Diagnostics.AddError("Error marshaling firewall rule", err.Error())
		return
	}

	ruleResp, err := r.client.CreateFirewallRule(ctx, plan.ServiceID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Error creating firewall rule", err.Error())
		return
	}

	plan.Position = types.Int64Value(int64(ruleResp.Position.Int64()))

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *firewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallRuleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Position.IsNull() {
		return
	}

	fw, err := r.client.GetFirewall(ctx, state.ServiceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading firewall", err.Error())
		return
	}

	if fw == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	found := false
	targetPos := state.Position.ValueInt64()
	for _, rule := range fw.Rules {
		if rule.Position.Int64() == targetPos {
			found = true
			state.Action = types.StringValue(rule.Action)
			state.Protocol = types.StringValue(rule.Protocol)
			state.Port = types.StringValue(rule.Port)
			state.Source = types.StringValue(rule.Source)
			state.Direction = types.StringValue(rule.Direction)
			state.Name = types.StringValue(rule.Name)
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

func (r *firewallRuleResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"SHC firewall rules cannot be updated. Recreate the resource to change its configuration.",
	)
}

func (r *firewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallRuleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteFirewallRule(ctx, state.ServiceID.ValueString(), state.Position.ValueInt64())
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			resp.Diagnostics.AddError("Error deleting firewall rule", err.Error())
			return
		}
	}
}

var _ resource.Resource = (*firewallRuleResource)(nil)
var _ resource.ResourceWithImportState = (*firewallRuleResource)(nil)
