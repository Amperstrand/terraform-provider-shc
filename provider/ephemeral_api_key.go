package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// apiKeyEventModel is the Open-time result of the ephemeral API-key
// resource (#30). The secret never reaches state — that is the entire
// point: keys are account-takeover-capable (a leaked key can read
// password-reset emails, toolkit lesson 24), so they belong in
// ephemeral resources, not plan/state files.
type apiKeyEventModel struct {
	ID             types.String `tfsdk:"id"`
	Label          types.String `tfsdk:"label"`
	Scope          types.String `tfsdk:"scope"`
	ExpiresInDays  types.Int64  `tfsdk:"expires_in_days"`
	ExpiresAt      types.String `tfsdk:"expires_at"`
	APIKey         types.String `tfsdk:"api_key"`
}

var _ ephemeral.EphemeralResourceWithConfigure = (*apiKeyEphemeralResource)(nil)

type apiKeyEphemeralResource struct {
	client *SHCClient
}

// NewAPIKeyEphemeralResource returns a fresh shc_api_key ephemeral resource.
func NewAPIKeyEphemeralResource() ephemeral.EphemeralResource {
	return &apiKeyEphemeralResource{}
}

func (r *apiKeyEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Mints a short-lived SHC API key without persisting it in state. " +
			"Requires provider account_username/account_password — key minting is Basic-auth-only " +
			"(Bearer keys are forbidden on POST /account/api-keys).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Numeric key id (revocation is Basic+OTP-only via the portal).",
			},
			"label": schema.StringAttribute{
				Required:    true,
				Description: "Key label, e.g. 'ci-temp'.",
			},
			"scope": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "'operate' (default — cannot spend) or 'full'. Spend-class ops (order, cancel, pay) need 'full'.",
			},
			"expires_in_days": schema.Int64Attribute{
				Optional:    true,
				Description: "Days until expiry (1-730). Prefer short expiries: keys are account-takeover-capable and cannot be revoked by another key.",
				Validators: []validator.Int64{
					int64validator.Between(1, 730),
				},
			},
			"expires_at": schema.StringAttribute{
				Computed:    true,
				Description: "Expiry timestamp reported by the API.",
			},
			"api_key": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The raw key (shc_live_...). Shown once; never persisted in state.",
			},
		},
	}
}

func (r *apiKeyEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*SHCClient)
	if !ok {
		resp.Diagnostics.AddError("unexpected provider data", "expected *SHCClient")
		return
	}
	r.client = client
}

func (r *apiKeyEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var config struct {
		ID            types.String `tfsdk:"id"`
		Label         types.String `tfsdk:"label"`
		Scope         types.String `tfsdk:"scope"`
		ExpiresInDays types.Int64  `tfsdk:"expires_in_days"`
		ExpiresAt     types.String `tfsdk:"expires_at"`
		APIKey        types.String `tfsdk:"api_key"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("provider not configured", "missing SHC client")
		return
	}
	// Identity-class endpoint: Basic-only. A Bearer key gets 403 here —
	// fail with the fix instead of a cryptic server error.
	if !r.client.UsingBasicAuth() {
		resp.Diagnostics.AddError(
			"shc_api_key requires account credentials",
			"Minting API keys is Basic-auth-only (Bearer keys are forbidden on POST /account/api-keys). "+
				"Set account_username (SHC_ACCOUNT_USER) and account_password (SHC_ACCOUNT_PASSWORD) on the provider. "+
				"The username is the BARE Blesta login, not necessarily the account email.",
		)
		return
	}

	scope := "operate"
	if !config.Scope.IsNull() && config.Scope.ValueString() != "" {
		scope = config.Scope.ValueString()
	}
	days := 0
	if !config.ExpiresInDays.IsNull() {
		days = int(config.ExpiresInDays.ValueInt64())
	}

	key, err := r.client.CreateAPIKey(ctx, config.Label.ValueString(), scope, days)
	if err != nil {
		resp.Diagnostics.AddError("creating api key", fmt.Sprintf("%v", err))
		return
	}

	event := apiKeyEventModel{
		ID:            types.StringValue(string(key.ID)),
		Label:         types.StringValue(orDefault(key.Label, config.Label.ValueString())),
		Scope:         types.StringValue(orDefault(key.Scope, scope)),
		ExpiresInDays: config.ExpiresInDays,
		ExpiresAt:     types.StringValue(key.ExpiresAt),
		APIKey:        types.StringValue(key.Secret()),
	}
	// Direct/unit callers pass a zero Result; the framework normally
	// pre-fills it with the schema + unknown raw value.
	if resp.Result.Schema == nil {
		var sr ephemeral.SchemaResponse
		r.Schema(ctx, ephemeral.SchemaRequest{}, &sr)
		resp.Result = tfsdk.EphemeralResultData{
			Raw:    tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), tftypes.UnknownValue),
			Schema: &sr.Schema,
		}
	}
	resp.Diagnostics.Append(resp.Result.Set(ctx, event)...)
}

func orDefault(v, d string) string {
	if v != "" {
		return v
	}
	return d
}
