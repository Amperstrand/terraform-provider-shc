package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	ID      types.String `tfsdk:"id"`
	Status  types.String `tfsdk:"status"`
	Zone    types.String `tfsdk:"zone"`
	Package types.String `tfsdk:"package"`
	VMs     []vmListItem `tfsdk:"vms"`
}

// vmItem is the parsed /vm list entry before framework typing.
type vmItem struct {
	ServiceID         string
	Hostname          string
	Status            string
	ProvisioningState string
	IP                string
	Package           string
}

// vmFilter selects VMs by exact status, derived zone, or case-insensitive
// package-name substring. Empty fields match everything.
type vmFilter struct {
	Status  string
	Zone    string
	Package string
}

// zoneForPackage maps a package name to its SHC site: Dev VPS plans live in
// Cherryvale, KS; every other line (NVMe/SSD/HDD) in Katy, TX.
func zoneForPackage(pkg string) string {
	if strings.HasPrefix(strings.ToLower(pkg), "dev ") {
		return "cherryvale"
	}
	return "katy"
}

func filterVMItems(items []vmItem, f vmFilter) []vmItem {
	var out []vmItem
	pkgNeedle := strings.ToLower(f.Package)
	for _, vm := range items {
		if f.Status != "" && vm.Status != f.Status {
			continue
		}
		if f.Zone != "" && zoneForPackage(vm.Package) != f.Zone {
			continue
		}
		if pkgNeedle != "" && !strings.Contains(strings.ToLower(vm.Package), pkgNeedle) {
			continue
		}
		out = append(out, vm)
	}
	return out
}

func NewVMsDataSource() datasource.DataSource {
	return &vmsDataSource{}
}

func (d *vmsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vms"
}

func (d *vmsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists all VMs on the SHC account, optionally filtered by zone, status, or package. Useful for inventory tracking and cost analysis.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The data source ID (always 'vms').",
			},
			"status": schema.StringAttribute{
				Optional:    true,
				Description: "Only include VMs with this exact service status (e.g. active, pending, canceled).",
			},
			"zone": schema.StringAttribute{
				Optional:    true,
				Description: "Only include VMs in this zone: katy (NVMe/SSD/HDD) or cherryvale (Dev VPS).",
				Validators: []validator.String{
					stringvalidator.OneOf("katy", "cherryvale"),
				},
			},
			"package": schema.StringAttribute{
				Optional:    true,
				Description: "Only include VMs whose package name contains this string, case-insensitive (e.g. \"dev\", \"NVMe VPS - Starter\").",
			},
			"vms": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The list of VMs on the account (after filters).",
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

func (d *vmsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config vmsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

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

	var items []vmItem
	for _, item := range listResp.Items {
		ip := ""
		if len(item.IPs) > 0 {
			ip = item.IPs[0].IP
		}
		items = append(items, vmItem{
			ServiceID:         item.ServiceID.String(),
			Hostname:          item.Hostname,
			Status:            item.Status,
			ProvisioningState: item.ProvisioningState,
			IP:                ip,
			Package:           item.Package,
		})
	}

	filter := vmFilter{
		Status:  config.Status.ValueString(),
		Zone:    config.Zone.ValueString(),
		Package: config.Package.ValueString(),
	}

	state := vmsDataSourceModel{
		ID:      types.StringValue("vms"),
		Status:  config.Status,
		Zone:    config.Zone,
		Package: config.Package,
	}
	for _, vm := range filterVMItems(items, filter) {
		state.VMs = append(state.VMs, vmListItem{
			ServiceID:         types.StringValue(vm.ServiceID),
			Hostname:          types.StringValue(vm.Hostname),
			Status:            types.StringValue(vm.Status),
			ProvisioningState: types.StringValue(vm.ProvisioningState),
			IP:                types.StringValue(vm.IP),
			Package:           types.StringValue(vm.Package),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
