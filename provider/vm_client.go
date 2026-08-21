package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func (c *SHCClient) SubmitOrder(ctx context.Context, hostname string, packageID, pricingID int64, configOptions map[string]string, sshKey string) (*OrderResponse, error) {
	c.orderIdempotencyKey = fmt.Sprintf("order-%d-%d", time.Now().UnixNano(), rand.Int64())
	defer func() { c.orderIdempotencyKey = "" }()

	orderFormID, err := c.resolveOrderFormID(ctx, packageID)
	if err != nil {
		return nil, fmt.Errorf("resolving order_form_id for package %d: %w", packageID, err)
	}

	orderReq := OrderRequest{
		Hostname:      hostname,
		PackageID:     packageID,
		PricingID:     pricingID,
		OrderFormID:   orderFormID,
		SSHKey:        sshKey,
		ConfigOptions: configOptions,
	}

	body, err := json.Marshal(orderReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling order request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, "/ordering/submit", body, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPost, "/ordering/submit", body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("order submission failed (status %d): %s", statusCode, string(respBody))
	}

	var fullResp struct {
		ServiceIDs []flexibleString `json:"service_ids"`
		ServiceID  flexibleString   `json:"service_id"`
		ID         flexibleString   `json:"id"`
		Invoice    struct {
			InvoiceID flexibleString `json:"invoice_id"`
		} `json:"invoice"`
		Next struct {
			PaymentRequired bool `json:"payment_required"`
		} `json:"next"`
	}
	unwrapped := unwrapData(respBody)
	if err := json.Unmarshal(unwrapped, &fullResp); err != nil {
		return nil, fmt.Errorf("parsing order response: %w (body: %s)", err, string(respBody))
	}

	orderResp := OrderResponse{
		ServiceIDs: fullResp.ServiceIDs,
		ServiceID:  fullResp.ServiceID,
		ID:         fullResp.ID,
	}

	if orderResp.ResolveServiceID() == "" {
		return nil, fmt.Errorf("order submission returned no service ID (body: %s)", string(respBody))
	}

	return &orderResp, nil
}

func (c *SHCClient) ResolveAddons(ctx context.Context, packageID int64, diskGB, ramMB, cpu types.Int64, template types.String) (map[string]string, error) {
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, "/ordering/catalog", nil, "")
	if err != nil {
		return nil, fmt.Errorf("fetching catalog: %w", err)
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("catalog fetch failed (status %d)", statusCode)
	}

	var catalogResp struct {
		Items []struct {
			PackageID           int64 `json:"package_id"`
			AvailableConfigOpts []struct {
				Options []struct {
					OptionID int64  `json:"option_id"`
					Name     string `json:"name"`
					Values   []struct {
						Value string `json:"value"`
					} `json:"values"`
				} `json:"options"`
			} `json:"available_config_options"`
		} `json:"items"`
	}

	unwrapped := unwrapData(respBody)
	if err := json.Unmarshal(unwrapped, &catalogResp); err != nil {
		return nil, fmt.Errorf("parsing catalog: %w", err)
	}

	var pkgOpts map[string]struct {
		optionID int64
		values   map[string]bool
	}
	for _, pkg := range catalogResp.Items {
		if pkg.PackageID != packageID {
			continue
		}
		pkgOpts = make(map[string]struct {
			optionID int64
			values   map[string]bool
		})
		for _, block := range pkg.AvailableConfigOpts {
			for _, opt := range block.Options {
				vals := make(map[string]bool)
				for _, v := range opt.Values {
					vals[v.Value] = true
				}
				pkgOpts[opt.Name] = struct {
					optionID int64
					values   map[string]bool
				}{optionID: opt.OptionID, values: vals}
			}
		}
		break
	}
	if pkgOpts == nil {
		return nil, fmt.Errorf("package_id %d not found in catalog", packageID)
	}

	result := make(map[string]string)
	specs := []struct {
		name     string
		value    string
		hasValue bool
	}{
		{"disk", strconv.FormatInt(diskGB.ValueInt64(), 10), !diskGB.IsNull()},
		{"ram", strconv.FormatInt(ramMB.ValueInt64(), 10), !ramMB.IsNull()},
		{"cpu", strconv.FormatInt(cpu.ValueInt64(), 10), !cpu.IsNull()},
		{"template", template.ValueString(), !template.IsNull()},
	}

	for _, spec := range specs {
		if !spec.hasValue {
			continue
		}
		opt, ok := pkgOpts[spec.name]
		if !ok {
			return nil, fmt.Errorf("package %d does not expose a '%s' option", packageID, spec.name)
		}
		if !opt.values[spec.value] {
			return nil, fmt.Errorf("package %d %s=%s not available", packageID, spec.name, spec.value)
		}
		result[strconv.FormatInt(opt.optionID, 10)] = spec.value
	}

	return result, nil
}

func (c *SHCClient) GetVM(ctx context.Context, serviceID string) (*VMResponse, error) {
	path := "/vm/" + serviceID

	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, ErrVMNotFound
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get VM failed (status %d): %s", statusCode, string(respBody))
	}

	var vmResp VMResponse
	unwrapped := unwrapData(respBody)
	if err := json.Unmarshal(unwrapped, &vmResp); err != nil {
		return nil, fmt.Errorf("parsing VM response: %w (body: %s)", err, string(respBody))
	}

	return &vmResp, nil
}

func (c *SHCClient) CancelVM(ctx context.Context, serviceID string, immediate bool) error {
	return retryOnLock(ctx, func() error {
		return c.cancelVMOnce(ctx, serviceID, immediate)
	})
}

func (c *SHCClient) cancelVMOnce(ctx context.Context, serviceID string, immediate bool) error {
	path := "/vm/" + serviceID + "/cancel"

	cancelReq := CancelRequest{Immediate: immediate}
	body, err := json.Marshal(cancelReq)
	if err != nil {
		return fmt.Errorf("marshaling cancel request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		return err
	}

	if statusCode == http.StatusNotFound {
		return nil
	}

	if statusCode >= 400 {
		return fmt.Errorf("cancel VM failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) ApplySSHKey(ctx context.Context, serviceID, sshKey string) error {
	path := "/vm/" + serviceID + "/ssh-keys/apply-live"

	body, err := json.Marshal(map[string]string{"ssh_key": sshKey})
	if err != nil {
		return fmt.Errorf("marshaling SSH key request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		return err
	}

	if statusCode >= 400 {
		return fmt.Errorf("apply SSH key failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) SetPowerState(ctx context.Context, serviceID, action string) error {
	path := "/vm/" + serviceID + "/" + action

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPatch, path, nil, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPatch, path, nil, respBody)
		return err
	}

	if statusCode >= 400 {
		return fmt.Errorf("set power state %s failed (status %d): %s", action, statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) UpgradeVM(ctx context.Context, serviceID string, pricingRef int64) error {
	const maxUpgradeRetries = 8
	const upgradeRetryDelay = 20 * time.Second

	body, _ := json.Marshal(map[string]interface{}{
		"pricing_ref":     pricingRef,
		"idempotency_key": fmt.Sprintf("tf-upgrade-%d", time.Now().UnixNano()),
	})

	var lastErr error
	for attempt := 0; attempt <= maxUpgradeRetries; attempt++ {
		path := "/vm/" + serviceID + "/upgrade"

		statusCode, respBody, err := c.doRequest(ctx, http.MethodPatch, path, body, "")
		if err != nil {
			return err
		}

		if statusCode == http.StatusConflict {
			errBody := string(respBody)
			if strings.Contains(errBody, "confirmation_id") {
				_, confirmErr := c.handleConfirmation(ctx, http.MethodPatch, path, body, respBody)
				if confirmErr == nil {
					return nil
				}
				if strings.Contains(confirmErr.Error(), "service_not_active") && attempt < maxUpgradeRetries {
					lastErr = confirmErr
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(upgradeRetryDelay):
					}
					continue
				}
				return confirmErr
			}
			if attempt < maxUpgradeRetries && strings.Contains(errBody, "service_not_active") {
				lastErr = fmt.Errorf("upgrade: service not active yet (attempt %d): %s", attempt+1, errBody)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(upgradeRetryDelay):
				}
				continue
			}
			return fmt.Errorf("upgrade conflict: %s", errBody)
		}

		if statusCode >= 400 {
			errBody := string(respBody)
			lastErr = fmt.Errorf("upgrade failed (status %d): %s", statusCode, errBody)

			if attempt < maxUpgradeRetries && strings.Contains(errBody, "service_not_active") {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(upgradeRetryDelay):
				}
				continue
			}
			return lastErr
		}

		return nil
	}
	return fmt.Errorf("upgrade failed after %d retries: %w", maxUpgradeRetries, lastErr)
}

func (c *SHCClient) ListUpgradeOptions(ctx context.Context, serviceID string) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/upgrade-options"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("list upgrade options failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) ListVMAddons(ctx context.Context, serviceID string) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/addons"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("list VM addons failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) GetVMAddonOptions(ctx context.Context, serviceID string) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/addons/options"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("get VM addon options failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) CreateVMAddon(ctx context.Context, serviceID string, body json.RawMessage) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/addons"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusConflict {
		return c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("create VM addon failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) PreviewVMAddon(ctx context.Context, serviceID string, body json.RawMessage) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/addons/preview"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("preview VM addon failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) GetVMTermOptions(ctx context.Context, serviceID string) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/term-options"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("get VM term options failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) ChangeVMTerm(ctx context.Context, serviceID string, body json.RawMessage) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/term"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusConflict {
		return c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("change VM term failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) PreviewVMTermChange(ctx context.Context, serviceID string, body json.RawMessage) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/term/preview"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("preview VM term change failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}
