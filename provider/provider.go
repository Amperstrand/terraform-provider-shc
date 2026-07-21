package provider

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.21.0 generate

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type SHCProvider struct {
	version string
}

type SHCProviderModel struct {
	APIKey   types.String `tfsdk:"api_key"`
	Endpoint types.String `tfsdk:"endpoint"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SHCProvider{
			version: version,
		}
	}
}

func (p *SHCProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "shc"
	resp.Version = p.version
}

func (p *SHCProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The SHC API key for authentication.",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "The SHC API base URL. Defaults to https://blesta.sovereignhybridcompute.com/user-api/v2.",
			},
		},
	}
}

func (p *SHCProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config SHCProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := config.APIKey.ValueString()
	endpoint := config.Endpoint.ValueString()

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing SHC API Key",
			"The provider cannot create the SHC API client without an API key. Set the api_key argument in the provider configuration.",
		)
		return
	}

	client := NewSHCClient(apiKey, endpoint)
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *SHCProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVMResource,
		NewSnapshotResource,
		NewBackupResource,
		NewFirewallRuleResource,
		NewRDNSResource,
	}
}

func (p *SHCProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewVMDataSource,
		NewCatalogDataSource,
		NewTemplatesDataSource,
		NewMachineTypesDataSource,
		NewEventsDataSource,
		NewBalanceDataSource,
	}
}

func (p *SHCProvider) Functions(_ context.Context) []func() function.Function {
	return []func() function.Function{}
}

var _ provider.Provider = (*SHCProvider)(nil)

func providerDataAssert(data any, name string) (*SHCClient, error) {
	client, ok := data.(*SHCClient)
	if !ok {
		return nil, fmt.Errorf("unexpected provider data type %T for %s", data, name)
	}
	return client, nil
}
