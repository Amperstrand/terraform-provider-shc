package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ function.Function = ParseSizeFunction{}

type ParseSizeFunction struct{}

func NewParseSizeFunction() function.Function {
	return &ParseSizeFunction{}
}

func (f ParseSizeFunction) Metadata(_ context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "parse_size"
}

func (f ParseSizeFunction) Definition(_ context.Context, _ function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary:     "Parse an SHC size name into its components",
		Description: "Converts a spec-encoding size name (e.g. nvme-2c-8gb) into CPU cores, RAM (MB), package ID, pricing ID, and daily price.",
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "size",
				Description: "The size name (e.g. nvme-2c-8gb, ssd-4c-16gb, dev-8c-32gb)",
			},
		},
		Return: function.ObjectReturn{
			AttributeTypes: map[string]attr.Type{
				"cpu":         types.Int64Type,
				"ram_mb":      types.Int64Type,
				"package_id":  types.Int64Type,
				"pricing_id":  types.Int64Type,
				"line":        types.StringType,
				"daily_price": types.Float64Type,
			},
		},
	}
}

func (f ParseSizeFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	var sizeName string
	req.Arguments.GetArgument(ctx, 0, &sizeName)

	pkgID, priceID, cpu, ramMB, _, line, dailyPrice, err := resolveSizeFull(sizeName)
	if err != nil {
		resp.Error = function.NewFuncError(err.Error())
		return
	}

	result, diags := types.ObjectValue(
		map[string]attr.Type{
			"cpu":         types.Int64Type,
			"ram_mb":      types.Int64Type,
			"package_id":  types.Int64Type,
			"pricing_id":  types.Int64Type,
			"line":        types.StringType,
			"daily_price": types.Float64Type,
		},
		map[string]attr.Value{
			"cpu":         types.Int64Value(cpu),
			"ram_mb":      types.Int64Value(ramMB),
			"package_id":  types.Int64Value(pkgID),
			"pricing_id":  types.Int64Value(priceID),
			"line":        types.StringValue(line),
			"daily_price": types.Float64Value(dailyPrice),
		},
	)
	if diags.HasError() {
		resp.Error = function.NewFuncError("failed to construct result object")
		return
	}
	resp.Result.Set(ctx, result)
}
