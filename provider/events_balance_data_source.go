package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ── Events data source ─────────────────────────────────────

type eventsDataSource struct {
	client *SHCClient
}

type eventsDataSourceModel struct {
	Limit  types.Int64  `tfsdk:"limit"`
	Events types.String `tfsdk:"events"`
}

func NewEventsDataSource() datasource.DataSource {
	return &eventsDataSource{}
}

func (d *eventsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_events"
}

func (d *eventsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches recent CloudEvents from the SHC events feed (lifecycle, billing, orders).",
		Attributes: map[string]schema.Attribute{
			"limit": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum number of events to return (default: 20).",
			},
			"events": schema.StringAttribute{
				Computed:    true,
				Description: "JSON-encoded array of CloudEvent objects.",
			},
		},
	}
}

func (d *eventsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*SHCClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected type", "Expected *SHCClient")
		return
	}
	d.client = client
}

func (d *eventsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state eventsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.client.ListEvents(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list events", err.Error())
		return
	}

	state.Events = types.StringValue(string(raw))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ── Balance data source ────────────────────────────────────

type balanceDataSource struct {
	client *SHCClient
}

type balanceEntryModel struct {
	Currency        types.String `tfsdk:"currency"`
	AvailableCredit types.String `tfsdk:"available_credit"`
}

type balanceDataSourceModel struct {
	Balances []balanceEntryModel `tfsdk:"balances"`
}

func NewBalanceDataSource() datasource.DataSource {
	return &balanceDataSource{}
}

func (d *balanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_balance"
}

func (d *balanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the current account balance and available credit.",
		Attributes: map[string]schema.Attribute{
			"balances": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Account balance entries by currency.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"currency": schema.StringAttribute{
							Computed:    true,
							Description: "Currency code (e.g., USD).",
						},
						"available_credit": schema.StringAttribute{
							Computed:    true,
							Description: "Available credit in the given currency.",
						},
					},
				},
			},
		},
	}
}

func (d *balanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*SHCClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected type", "Expected *SHCClient")
		return
	}
	d.client = client
}

func (d *balanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	result, err := d.client.GetBalance(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get balance", err.Error())
		return
	}

	var state balanceDataSourceModel
	for _, b := range result.Balances {
		state.Balances = append(state.Balances, balanceEntryModel{
			Currency:        types.StringValue(b.Currency),
			AvailableCredit: types.StringValue(b.AvailableCredit),
		})
	}
	if len(state.Balances) == 0 {
		state.Balances = []balanceEntryModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
