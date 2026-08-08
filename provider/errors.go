package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

type shcAPIError struct {
	Error shcAPIErrorBody `json:"error"`
}

type shcAPIErrorBody struct {
	Code      string             `json:"code"`
	Message   string             `json:"message"`
	Details   []shcAPIErrorField `json:"details"`
	ErrorCode string             `json:"error_code"`
}

type shcAPIErrorField struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

func parseSHCError(body []byte) *shcAPIError {
	var apiErr shcAPIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return nil
	}
	if apiErr.Error.Code == "" && apiErr.Error.Message == "" {
		return nil
	}
	return &apiErr
}

func shcErrorDiagnostic(operation string, body []byte) (diag.Diagnostic, bool) {
	apiErr := parseSHCError(body)
	if apiErr == nil {
		return nil, false
	}

	summary := fmt.Sprintf("SHC API: %s failed", operation)
	if apiErr.Error.Code != "" {
		summary = fmt.Sprintf("%s (%s)", apiErr.Error.Message, apiErr.Error.Code)
	}

	var detail strings.Builder
	detail.WriteString(apiErr.Error.Message)

	if len(apiErr.Error.Details) > 0 {
		detail.WriteString("\n\nField details:")
		for _, d := range apiErr.Error.Details {
			detail.WriteString(fmt.Sprintf("\n  • %s: %s", d.Field, d.Issue))
		}
	}

	return diag.NewErrorDiagnostic(summary, detail.String()), true
}

func addSHCError(diags *diag.Diagnostics, operation string, err error) {
	if err == nil {
		return
	}
	errStr := err.Error()
	
	start := strings.Index(errStr, "{")
	if start >= 0 {
		body := []byte(errStr[start:])
		if d, ok := shcErrorDiagnostic(operation, body); ok {
			diags.Append(d)
			return
		}
	}
	diags.AddError(
		fmt.Sprintf("%s failed", operation),
		err.Error(),
	)
}
