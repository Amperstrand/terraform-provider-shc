package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type machineTypesDataSource struct {
	client *SHCClient
}

type machineTypeModel struct {
	Name         types.String `tfsdk:"name"`
	PackageID    types.Int64  `tfsdk:"package_id"`
	CPU          types.Int64  `tfsdk:"cpu"`
	MemoryMB     types.Int64  `tfsdk:"memory_mb"`
	DiskGB       types.Int64  `tfsdk:"disk_gb"`
	PriceDaily   types.String `tfsdk:"price_daily"`
	PriceWeekly  types.String `tfsdk:"price_weekly"`
	PriceMonthly types.String `tfsdk:"price_monthly"`
}

type machineTypesDataSourceModel struct {
	MachineTypes []machineTypeModel `tfsdk:"machine_types"`
}

func NewMachineTypesDataSource() datasource.DataSource {
	return &machineTypesDataSource{}
}

func (d *machineTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_types"
}

func (d *machineTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the SHC ordering catalog, listing available VPS machine types with specs and pricing.",
		Attributes: map[string]schema.Attribute{
			"machine_types": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The list of available VPS machine types with resource specs and pricing.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The human-readable name of the machine type.",
						},
						"package_id": schema.Int64Attribute{
							Computed:    true,
							Description: "The SHC package ID.",
						},
						"cpu": schema.Int64Attribute{
							Computed:    true,
							Description: "The number of CPU cores.",
						},
						"memory_mb": schema.Int64Attribute{
							Computed:    true,
							Description: "The amount of memory in megabytes.",
						},
						"disk_gb": schema.Int64Attribute{
							Computed:    true,
							Description: "The amount of disk space in gigabytes.",
						},
						"price_daily": schema.StringAttribute{
							Computed:    true,
							Description: "The daily price for this machine type, if available.",
						},
						"price_weekly": schema.StringAttribute{
							Computed:    true,
							Description: "The weekly price for this machine type, if available.",
						},
						"price_monthly": schema.StringAttribute{
							Computed:    true,
							Description: "The monthly price for this machine type, if available.",
						},
					},
				},
			},
		},
	}
}

func (d *machineTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_machine_types data source")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	d.client = client
}

func priceForPeriod(pricing []CatalogPricingResponse, period string) string {
	for _, p := range pricing {
		if p.Period == period {
			return p.Price.String()
		}
	}
	return ""
}

func (d *machineTypesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state machineTypesDataSourceModel

	packages, err := d.client.GetCatalog(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading catalog", err.Error())
		return
	}

	for _, pkg := range packages {
		state.MachineTypes = append(state.MachineTypes, machineTypeModel{
			Name:         types.StringValue(pkg.Name),
			PackageID:    types.Int64Value(pkg.PackageID),
			CPU:          types.Int64Value(pkg.CPU),
			MemoryMB:     types.Int64Value(pkg.MemoryMB),
			DiskGB:       types.Int64Value(pkg.DiskGB),
			PriceDaily:   types.StringValue(priceForPeriod(pkg.Pricing, "day")),
			PriceWeekly:  types.StringValue(priceForPeriod(pkg.Pricing, "week")),
			PriceMonthly: types.StringValue(priceForPeriod(pkg.Pricing, "month")),
		})
	}

	if state.MachineTypes == nil {
		state.MachineTypes = []machineTypeModel{}
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

var _ datasource.DataSource = (*machineTypesDataSource)(nil)
