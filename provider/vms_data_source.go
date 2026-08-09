package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type vmsDataSource struct {
	client *SHCClient
}

type vmListItem struct {
	ServiceID         types.String `tfsdk:"service_id"`
	Hostname          types.String `tfsdk:"hostname"`
	Status            types.String `tfsdk:"status"`
	ProvisioningState types.String `tfsdk:"provisioning_state"`
	IP                types.String `tfsdk:"ip"`
	Package           types.String `tfsdk:"package"`
}

type vmsDataSourceModel struct {
	VMs []vmListItem `tfsdk:"vms"`
}

func NewVMsDataSource() datasource.DataSource {
	return &vmsDataSource{}
}

func (d *vmsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vms"
}

func (d *vmsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all VMs on the SHC account. Useful for inventory tracking and cost analysis.",
		Attributes: map[string]schema.Attribute{
			"vms": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The list of VMs on the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"service_id": schema.StringAttribute{
							Computed:    true,
							Description: "The SHC service ID.",
						},
						"hostname": schema.StringAttribute{
							Computed:    true,
							Description: "The VM hostname.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "The service status (active, pending, canceled).",
						},
						"provisioning_state": schema.StringAttribute{
							Computed:    true,
							Description: "The provisioning state.",
						},
						"ip": schema.StringAttribute{
							Computed:    true,
							Description: "The primary IP address.",
						},
						"package": schema.StringAttribute{
							Computed:    true,
							Description: "The package name (e.g. NVMe VPS - Starter).",
						},
					},
				},
			},
		},
	}
}

func (d *vmsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := providerDataAssert(req.ProviderData, "shc_vms data source")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	d.client = client
}

func (d *vmsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	statusCode, respBody, err := d.client.doRequest(ctx, http.MethodGet, "/vm", nil, "")
	if err != nil {
		resp.Diagnostics.AddError("Error listing VMs", err.Error())
		return
	}
	if statusCode >= 400 {
		resp.Diagnostics.AddError("Error listing VMs", fmt.Sprintf("API returned status %d: %s", statusCode, string(respBody)))
		return
	}

	var listResp struct {
		Items []struct {
			ServiceID         flexibleString `json:"service_id"`
			Hostname          string         `json:"hostname"`
			Status            string         `json:"service_status"`
			ProvisioningState string         `json:"provisioning_state"`
			IPs               []vmIP         `json:"ips"`
			Package           string         `json:"package"`
		} `json:"items"`
	}
	if err := json.Unmarshal(unwrapData(respBody), &listResp); err != nil {
		resp.Diagnostics.AddError("Error parsing VM list", err.Error())
		return
	}

	var state vmsDataSourceModel
	for _, item := range listResp.Items {
		ip := ""
		if len(item.IPs) > 0 {
			ip = item.IPs[0].IP
		}
		state.VMs = append(state.VMs, vmListItem{
			ServiceID:         types.StringValue(item.ServiceID.String()),
			Hostname:          types.StringValue(item.Hostname),
			Status:            types.StringValue(item.Status),
			ProvisioningState: types.StringValue(item.ProvisioningState),
			IP:                types.StringValue(ip),
			Package:           types.StringValue(item.Package),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
