package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// ---------------------------------------------------------------------------
// Int64 validators
// ---------------------------------------------------------------------------

// positiveInt64Validator validates that an int64 value is strictly positive (> 0).
type positiveInt64Validator struct{}

func (v positiveInt64Validator) Description(_ context.Context) string {
	return "value must be a positive integer"
}

func (v positiveInt64Validator) MarkdownDescription(_ context.Context) string {
	return v.Description(context.Background())
}

func (v positiveInt64Validator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueInt64()
	if val <= 0 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Integer Value",
			fmt.Sprintf("Expected a positive integer, got: %d", val),
		)
	}
}

// positiveInt64 returns a validator that rejects zero and negative integers.
func positiveInt64() positiveInt64Validator {
	return positiveInt64Validator{}
}

// ---------------------------------------------------------------------------
// String validators
// ---------------------------------------------------------------------------

// powerStateValidator validates that a string is "running" or "stopped".
type powerStateValidator struct{}

func (v powerStateValidator) Description(_ context.Context) string {
	return "value must be one of: running, stopped"
}

func (v powerStateValidator) MarkdownDescription(_ context.Context) string {
	return v.Description(context.Background())
}

func (v powerStateValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if val != "running" && val != "stopped" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Power State",
			fmt.Sprintf("Expected power_state to be 'running' or 'stopped', got: %q", val),
		)
	}
}

// powerState returns a validator that ensures the string is "running" or "stopped".
func powerState() powerStateValidator {
	return powerStateValidator{}
}
