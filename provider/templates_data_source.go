package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type templatesDataSource struct {
	client *SHCClient
}

type templateModel struct {
	Name   types.String `tfsdk:"name"`
	Family types.String `tfsdk:"family"`
	Arch   types.String `tfsdk:"arch"`
	Status types.String `tfsdk:"status"`
}

type templatesDataSourceModel struct {
	Templates []templateModel `tfsdk:"templates"`
}

func NewTemplatesDataSource() datasource.DataSource {
	return &templatesDataSource{}
}

func (d *templatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_templates"
}

func (d *templatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of available OS templates for SHC VPS instances.",
		Attributes: map[string]schema.Attribute{
			"templates": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The list of available OS templates.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The template name (e.g. debian13-cloud).",
						},
						"family": schema.StringAttribute{
							Computed:    true,
							Description: "The OS family (e.g. debian, ubuntu, fedora).",
						},
						"arch": schema.StringAttribute{
							Computed:    true,
							Description: "The CPU architecture (e.g. x86_64, aarch64).",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "The availability status of the template.",
						},
					},
				},
			},
		},
	}
}

func (d *templatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := providerDataAssert(req.ProviderData, "shc_templates data source")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	d.client = client
}

func (d *templatesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state templatesDataSourceModel

	templates, err := d.client.GetTemplates(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading templates", err.Error())
		return
	}

	for _, tmpl := range templates {
		state.Templates = append(state.Templates, templateModel{
			Name:   types.StringValue(tmpl.Name),
			Family: types.StringValue(tmpl.Family),
			Arch:   types.StringValue(tmpl.Arch),
			Status: types.StringValue(tmpl.Status),
		})
	}

	if state.Templates == nil {
		state.Templates = []templateModel{}
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

var _ datasource.DataSource = (*templatesDataSource)(nil)
