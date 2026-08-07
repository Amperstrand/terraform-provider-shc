package provider

import (
	"context"
	"fmt"
	"regexp"

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

// ---------------------------------------------------------------------------
// Hostname validator (RFC 1123): regex is inherently opaque, so documented.
// ---------------------------------------------------------------------------

// hostnameRegex: 1-63 chars, lowercase alphanumeric + hyphens, must start/end alphanumeric.
var hostnameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type hostnameValidator struct{}

func (v hostnameValidator) Description(_ context.Context) string {
	return "value must be a valid hostname: 1-63 chars, lowercase alphanumeric and hyphens, starting and ending with alphanumeric"
}

func (v hostnameValidator) MarkdownDescription(_ context.Context) string {
	return v.Description(context.Background())
}

func (v hostnameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if !hostnameRegex.MatchString(val) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Hostname",
			fmt.Sprintf("Expected a valid hostname (1-63 chars, lowercase alphanumeric and hyphens, starting and ending with alphanumeric), got: %q", val),
		)
	}
}

func hostname() hostnameValidator {
	return hostnameValidator{}
}

// ---------------------------------------------------------------------------
// Size validator: regex documents the accepted {line}-{cpu}c-{ram}gb pattern.
// ---------------------------------------------------------------------------

// sizeRegex: matches nvme-2c-8gb, dev-4c-16gb, ssd-1c-4gb, hdd-1c-2gb, etc.
var sizeRegex = regexp.MustCompile(`^(nvme|ssd|hdd|dev)-[1-9][0-9]*c-[1-9][0-9]*gb$`)

type sizeValidator struct{}

func (v sizeValidator) Description(_ context.Context) string {
	return "value must be a valid size name: {line}-{cpu}c-{ram}gb (e.g. nvme-2c-8gb, dev-4c-16gb)"
}

func (v sizeValidator) MarkdownDescription(_ context.Context) string {
	return v.Description(context.Background())
}

func (v sizeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if !sizeRegex.MatchString(val) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Size",
			fmt.Sprintf("Expected a size name in the format {line}-{cpu}c-{ram}gb (e.g. nvme-2c-8gb, dev-4c-16gb, ssd-1c-4gb), got: %q", val),
		)
	}
}

func sizeValidatorFn() sizeValidator {
	return sizeValidator{}
}
