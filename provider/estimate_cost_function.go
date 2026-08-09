package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ function.Function = EstimateCostFunction{}

type EstimateCostFunction struct{}

func NewEstimateCostFunction() function.Function {
	return &EstimateCostFunction{}
}

func (f EstimateCostFunction) Metadata(_ context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "estimate_cost"
}

func (f EstimateCostFunction) Definition(_ context.Context, _ function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary:     "Estimate the cost of an SHC VPS plan",
		Description: "Calculates total cost based on size name, duration, and unit (days, weeks, months, hours). Uses current SHC pricing.",
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "size",
				Description: "The size name (e.g. nvme-2c-8gb, dev-4c-16gb)",
			},
			function.Int64Parameter{
				Name:        "duration",
				Description: "The number of units (e.g. 30 for 30 days, 4 for 4 weeks)",
			},
			function.StringParameter{
				Name:        "unit",
				Description: "The time unit: 'hours', 'days', 'weeks', or 'months'. Default: 'days'.",
			},
		},
		Return: function.ObjectReturn{
			AttributeTypes: map[string]attr.Type{
				"daily_price":  types.Float64Type,
				"total_cost":   types.Float64Type,
				"unit":         types.StringType,
				"duration":     types.Int64Type,
				"hourly_price": types.Float64Type,
			},
		},
	}
}

func (f EstimateCostFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	var sizeName string
	var duration int64
	var unit string

	req.Arguments.GetArgument(ctx, 0, &sizeName)
	req.Arguments.GetArgument(ctx, 1, &duration)
	req.Arguments.GetArgument(ctx, 2, &unit)

	if unit == "" {
		unit = "days"
	}

	_, _, _, _, _, _, dailyPrice, err := resolveSizeFull(sizeName)
	if err != nil {
		resp.Error = function.NewFuncError(err.Error())
		return
	}

	hourlyPrice := dailyPrice / 24.0
	var totalCost float64

	switch unit {
	case "hours":
		totalCost = hourlyPrice * float64(duration)
	case "days":
		totalCost = dailyPrice * float64(duration)
	case "weeks":
		totalCost = dailyPrice * 7.0 * float64(duration)
	case "months":
		totalCost = dailyPrice * 30.0 * float64(duration)
	default:
		resp.Error = function.NewFuncError(fmt.Sprintf("invalid unit '%s': must be hours, days, weeks, or months", unit))
		return
	}

	result, diags := types.ObjectValue(
		map[string]attr.Type{
			"daily_price":  types.Float64Type,
			"total_cost":   types.Float64Type,
			"unit":         types.StringType,
			"duration":     types.Int64Type,
			"hourly_price": types.Float64Type,
		},
		map[string]attr.Value{
			"daily_price":  types.Float64Value(dailyPrice),
			"total_cost":   types.Float64Value(totalCost),
			"unit":         types.StringValue(unit),
			"duration":     types.Int64Value(duration),
			"hourly_price": types.Float64Value(hourlyPrice),
		},
	)
	if diags.HasError() {
		resp.Error = function.NewFuncError("failed to construct result")
		return
	}
	resp.Result.Set(ctx, result)
}
