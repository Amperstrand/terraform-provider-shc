package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
)

func (c *SHCClient) GetBalance(ctx context.Context) (*BalanceResponse, error) {
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, "/billing/balance", nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, fmt.Errorf("balance endpoint not found")
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get balance failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)
	var bal BalanceResponse
	if err := json.Unmarshal(unwrapped, &bal); err != nil {
		return nil, fmt.Errorf("parsing balance response: %w (body: %s)", err, string(respBody))
	}

	return &bal, nil
}

func (c *SHCClient) CheckCredit(ctx context.Context, minRequired float64) error {
	bal, err := c.GetBalance(ctx)
	if err != nil {
		return fmt.Errorf("credit check failed (cannot verify balance): %w", err)
	}
	var available float64
	for _, b := range bal.Balances {
		if b.Currency == "USD" {
			available, _ = strconv.ParseFloat(b.AvailableCredit, 64)
		}
	}
	if available < minRequired {
		return fmt.Errorf("insufficient credit: need $%.2f, have $%.2f. Add credit at https://blesta.sovereignhybridcompute.com/client/", minRequired, available)
	}
	return nil
}

func (c *SHCClient) SafeCredit(ctx context.Context) float64 {
	bal, err := c.GetBalance(ctx)
	if err != nil {
		return -1
	}
	for _, b := range bal.Balances {
		if b.Currency == "USD" {
			f, _ := strconv.ParseFloat(b.AvailableCredit, 64)
			return f
		}
	}
	return -1
}

func (c *SHCClient) EstimateDailyCost(ctx context.Context, packageID int64) float64 {
	if price, ok := dailyPriceForPackage(packageID); ok {
		return price
	}
	return 0
}

func (c *SHCClient) LedgerRefund(ctx context.Context, serviceID int64) *float64 {
	path := "/vm/" + strconv.FormatInt(serviceID, 10) + "/payments"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil || statusCode >= 400 {
		return nil
	}

	unwrapped := unwrapData(respBody)
	var paymentsResp struct {
		Items []struct {
			Total  flexibleString `json:"total"`
			Paid   flexibleString `json:"paid"`
			Amount flexibleString `json:"amount"`
		} `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &paymentsResp); err != nil {
		return nil
	}

	var totalRefund float64
	found := false
	for _, p := range paymentsResp.Items {
		amountStr := p.Amount.String()
		if amountStr == "" {
			amountStr = p.Total.String()
		}
		val, _ := strconv.ParseFloat(amountStr, 64)
		if val < 0 {
			totalRefund += math.Abs(val)
			found = true
		}
	}
	if !found {
		zero := 0.0
		return &zero
	}
	return &totalRefund
}

func (c *SHCClient) GetCatalog(ctx context.Context) ([]CatalogPackageResponse, error) {
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, "/ordering/catalog", nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, fmt.Errorf("catalog endpoint not found")
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get catalog failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var packages []CatalogPackageResponse
	if err := json.Unmarshal(unwrapped, &packages); err == nil && packages != nil {
		return packages, nil
	}

	var items struct {
		Items []CatalogPackageResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return items.Items, nil
	}

	var wrapped struct {
		Packages []CatalogPackageResponse `json:"packages"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Packages != nil {
		return wrapped.Packages, nil
	}

	return nil, fmt.Errorf("unable to parse catalog response: %s", string(respBody))
}

func (c *SHCClient) GetTemplates(ctx context.Context) ([]TemplateResponse, error) {
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, "/vm/templates", nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, fmt.Errorf("templates endpoint not found")
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get templates failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var templates []TemplateResponse
	if err := json.Unmarshal(unwrapped, &templates); err == nil && templates != nil {
		return templates, nil
	}

	var items struct {
		Items []TemplateResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return items.Items, nil
	}

	var wrapped struct {
		Templates []TemplateResponse `json:"templates"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Templates != nil {
		return wrapped.Templates, nil
	}

	return nil, fmt.Errorf("unable to parse templates response: %s", string(respBody))
}
