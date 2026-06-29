package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type catalogDataSource struct {
	client *SHCClient
}

type catalogPackageModel struct {
	PackageID types.Int64  `tfsdk:"package_id"`
	Name      types.String `tfsdk:"name"`
	CPU       types.Int64  `tfsdk:"cpu"`
	MemoryMB  types.Int64  `tfsdk:"memory_mb"`
	DiskGB    types.Int64  `tfsdk:"disk_gb"`
}

type catalogDataSourceModel struct {
	Packages []catalogPackageModel `tfsdk:"packages"`
}

func NewCatalogDataSource() datasource.DataSource {
	return &catalogDataSource{}
}

func (d *catalogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog"
}

func (d *catalogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the SHC ordering catalog, listing available VPS packages and their resource specifications.",
		Attributes: map[string]schema.Attribute{
			"packages": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The list of available VPS packages in the SHC catalog.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"package_id": schema.Int64Attribute{
							Computed:    true,
							Description: "The SHC package ID.",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The human-readable name of the package.",
						},
						"cpu": schema.Int64Attribute{
							Computed:    true,
							Description: "The number of CPU cores allocated to the package.",
						},
						"memory_mb": schema.Int64Attribute{
							Computed:    true,
							Description: "The amount of memory in megabytes.",
						},
						"disk_gb": schema.Int64Attribute{
							Computed:    true,
							Description: "The amount of disk space in gigabytes.",
						},
					},
				},
			},
		},
	}
}

func (d *catalogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_catalog data source")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	d.client = client
}

func (d *catalogDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state catalogDataSourceModel

	packages, err := d.client.GetCatalog(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading catalog", err.Error())
		return
	}

	for _, pkg := range packages {
		state.Packages = append(state.Packages, catalogPackageModel{
			PackageID: types.Int64Value(pkg.PackageID),
			Name:      types.StringValue(pkg.Name),
			CPU:       types.Int64Value(pkg.CPU),
			MemoryMB:  types.Int64Value(pkg.MemoryMB),
			DiskGB:    types.Int64Value(pkg.DiskGB),
		})
	}

	if state.Packages == nil {
		state.Packages = []catalogPackageModel{}
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

var _ datasource.DataSource = (*catalogDataSource)(nil)
