package provider

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.21.0 generate

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/metaschema"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type SHCProvider struct {
	version string
}

type SHCProviderModel struct {
	APIKey       types.String  `tfsdk:"api_key"`
	Endpoint     types.String  `tfsdk:"endpoint"`
	TimeoutSecs  types.Int64   `tfsdk:"timeout_seconds"`
	MaxRetries   types.Int64   `tfsdk:"max_retries"`
	RateLimitRPS types.Float64 `tfsdk:"rate_limit_rps"`
}

// maxRetriesOption maps the schema value to the client option: unset/null
// keeps the library default; an explicit 0 disables retries.
func maxRetriesOption(v types.Int64) int {
	if v.IsNull() {
		return 0
	}
	if v.ValueInt64() == 0 {
		return -1
	}
	return int(v.ValueInt64())
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
			"timeout_seconds": schema.Int64Attribute{
				Optional:    true,
				Description: "Per-request HTTP timeout in seconds. Defaults to 60.",
				Validators: []validator.Int64{
					int64validator.Between(1, 300),
				},
			},
			"max_retries": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum retries for retryable responses (429/5xx) per request. Defaults to 3; set 0 to disable retries.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
					int64validator.AtMost(10),
				},
			},
			"rate_limit_rps": schema.Float64Attribute{
				Optional:    true,
				Description: "Client-wide rate limit in requests per second (e.g. 2.5). Defaults to unlimited.",
				Validators: []validator.Float64{
					float64validator.AtLeast(0.1),
				},
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

	client := NewSHCClientWithOptions(apiKey, endpoint, ClientOptions{
		Timeout:      time.Duration(config.TimeoutSecs.ValueInt64()) * time.Second,
		MaxRetries:   maxRetriesOption(config.MaxRetries),
		RateLimitRPS: config.RateLimitRPS.ValueFloat64(),
	})
	client.SetUserAgent(fmt.Sprintf("terraform-provider-shc/%s (SHC API v2.4.24)", p.version))
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
		NewVMsDataSource,
		NewBillingDataSource,
		NewCatalogDataSource,
		NewTemplatesDataSource,
		NewMachineTypesDataSource,
		NewEventsDataSource,
		NewBalanceDataSource,
	}
}

func (p *SHCProvider) Functions(_ context.Context) []func() function.Function {
	return []func() function.Function{
		NewParseSizeFunction,
		NewEstimateCostFunction,
	}
}

var _ provider.Provider = (*SHCProvider)(nil)
var _ provider.ProviderWithMetaSchema = (*SHCProvider)(nil)

func (p *SHCProvider) MetaSchema(_ context.Context, _ provider.MetaSchemaRequest, resp *provider.MetaSchemaResponse) {
	resp.Schema = metaschema.Schema{
		Attributes: map[string]metaschema.Attribute{
			"schema_version": metaschema.Int64Attribute{
				Optional:    true,
				Description: "Tracks the provider schema version that wrote the state. Used for upgrade detection.",
			},
		},
	}
}

func providerDataAssert(data any, name string) (*SHCClient, error) {
	client, ok := data.(*SHCClient)
	if !ok {
		return nil, fmt.Errorf("unexpected provider data type %T for %s", data, name)
	}
	return client, nil
}
