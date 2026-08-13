package provider

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type billingDataSource struct {
	client *SHCClient
}

type billingModel struct {
	Balance  types.String  `tfsdk:"balance"`
	Credit   types.String  `tfsdk:"credit"`
	Currency types.String  `tfsdk:"currency"`
	Invoices []invoiceItem `tfsdk:"invoices"`
}

type invoiceItem struct {
	InvoiceID   types.String `tfsdk:"invoice_id"`
	Status      types.String `tfsdk:"status"`
	Total       types.String `tfsdk:"total"`
	DateDue     types.String `tfsdk:"date_due"`
	DateCreated types.String `tfsdk:"date_created"`
}

func NewBillingDataSource() datasource.DataSource {
	return &billingDataSource{}
}

func (d *billingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing"
}

func (d *billingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches account billing information: current balance, credit, and recent invoices.",
		Attributes: map[string]schema.Attribute{
			"balance": schema.StringAttribute{
				Computed:    true,
				Description: "The current account balance.",
			},
			"credit": schema.StringAttribute{
				Computed:    true,
				Description: "Available credit.",
			},
			"currency": schema.StringAttribute{
				Computed:    true,
				Description: "The account currency (e.g. USD).",
			},
			"invoices": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Recent invoices.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"invoice_id":   schema.StringAttribute{Computed: true, Description: "The invoice ID."},
						"status":       schema.StringAttribute{Computed: true, Description: "Invoice status (open, paid, etc.)."},
						"total":        schema.StringAttribute{Computed: true, Description: "The invoice total."},
						"date_due":     schema.StringAttribute{Computed: true, Description: "Date the invoice is due."},
						"date_created": schema.StringAttribute{Computed: true, Description: "Date the invoice was created."},
					},
				},
			},
		},
	}
}

func (d *billingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := providerDataAssert(req.ProviderData, "shc_billing data source")
	if err != nil {
		resp.Diagnostics.AddError("Provider Configuration Error", err.Error())
		return
	}
	d.client = client
}

func (d *billingDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Get balance
	balStatusCode, balBody, err := d.client.doRequest(ctx, http.MethodGet, "/billing/balance", nil, "")
	if err != nil {
		resp.Diagnostics.AddError("Error fetching balance", err.Error())
		return
	}

	var state billingModel

	if balStatusCode < 400 {
		var balResp struct {
			Credit []struct {
				Amount string `json:"amount"`
			} `json:"credit"`
			Currency string `json:"currency"`
		}
		if json.Unmarshal(unwrapData(balBody), &balResp) == nil {
			if len(balResp.Credit) > 0 {
				state.Credit = types.StringValue(balResp.Credit[0].Amount)
			}
			if balResp.Currency != "" {
				state.Currency = types.StringValue(balResp.Currency)
			}
		}
	}

	// Get invoices
	invStatusCode, invBody, err := d.client.doRequest(ctx, http.MethodGet, "/invoices", nil, "")
	if err == nil && invStatusCode < 400 {
		var invResp struct {
			Items []struct {
				InvoiceID   flexibleString `json:"invoice_id"`
				Status      string         `json:"invoice_status"`
				Total       string         `json:"total"`
				DateDue     string         `json:"date_due"`
				DateCreated string         `json:"date_created"`
			} `json:"items"`
		}
		if json.Unmarshal(unwrapData(invBody), &invResp) == nil {
			for _, inv := range invResp.Items {
				if len(state.Invoices) >= 20 {
					break
				}
				state.Invoices = append(state.Invoices, invoiceItem{
					InvoiceID:   types.StringValue(inv.InvoiceID.String()),
					Status:      types.StringValue(inv.Status),
					Total:       types.StringValue(inv.Total),
					DateDue:     types.StringValue(inv.DateDue),
					DateCreated: types.StringValue(inv.DateCreated),
				})
			}
		}
	}

	if state.Balance.IsNull() {
		state.Balance = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
