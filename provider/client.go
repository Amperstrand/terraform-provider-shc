package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultBaseURL = "https://blesta.sovereignhybridcompute.com/user-api/v2"

var ErrVMNotFound = errors.New("vm not found")

const (
	lockRetryDelay  = 5 * time.Second
	lockMaxRetries  = 3
)

func isVMLockedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "locked")
}

func retryOnLock(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= lockMaxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !isVMLockedErr(lastErr) {
			return lastErr
		}
		if attempt < lockMaxRetries {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while waiting for locked VM: %w", lastErr)
			case <-time.After(lockRetryDelay):
			}
		}
	}
	return fmt.Errorf("VM is locked by a running job after %d retries: %w", lockMaxRetries, lastErr)
}

func retryOnLockValue[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	for attempt := 0; attempt <= lockMaxRetries; attempt++ {
		result, lastErr = fn()
		if lastErr == nil {
			return result, nil
		}
		if !isVMLockedErr(lastErr) {
			return result, lastErr
		}
		if attempt < lockMaxRetries {
			select {
			case <-ctx.Done():
				return result, fmt.Errorf("context cancelled while waiting for locked VM: %w", lastErr)
			case <-time.After(lockRetryDelay):
			}
		}
	}
	return result, fmt.Errorf("VM is locked by a running job after %d retries: %w", lockMaxRetries, lastErr)
}

type SHCClient struct {
	baseURL     string
	apiKey      string
	httpClient  *http.Client
	costTracker *CostTracker
}

func NewSHCClient(apiKey, endpoint string) *SHCClient {
	if endpoint == "" {
		endpoint = defaultBaseURL
	}

	c := &SHCClient{
		baseURL: strings.TrimRight(endpoint, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	c.costTracker = NewCostTracker(c)
	return c
}

func stripNonJSONPrefix(data []byte) []byte {
	for i, b := range data {
		if b == '{' {
			return data[i:]
		}
	}
	return data
}

func unwrapData(raw []byte) []byte {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &wrapper) == nil && len(wrapper.Data) > 0 {
		return wrapper.Data
	}
	return raw
}

func (c *SHCClient) doRequest(ctx context.Context, method, path string, body []byte, confirmID string) (int, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return 0, nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if confirmID != "" {
		req.Header.Set("X-User-Api-Confirm", confirmID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("making request to %s: %w", path, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading response body: %w", err)
	}

	clean := stripNonJSONPrefix(raw)
	return resp.StatusCode, clean, nil
}

func (c *SHCClient) handleConfirmation(ctx context.Context, method, path string, body []byte, conflictBody []byte) ([]byte, error) {
	var conf confirmationResponse
	if err := json.Unmarshal(conflictBody, &conf); err != nil {
		return nil, fmt.Errorf("parsing 409 confirmation response: %w (body: %s)", err, string(conflictBody))
	}

	if conf.GetConfirmationID() == "" {
		return nil, fmt.Errorf("confirmation required but no confirmation_id in response: %s", string(conflictBody))
	}

	statusCode, respBody, err := c.doRequest(ctx, method, path, body, conf.GetConfirmationID())
	if err != nil {
		return nil, err
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("confirmed request to %s failed (status %d): %s", path, statusCode, string(respBody))
	}

	return respBody, nil
}

func (c *SHCClient) SubmitOrder(ctx context.Context, hostname string, packageID, pricingID int64, configOptions map[string]string) (*OrderResponse, error) {
	orderReq := OrderRequest{
		Hostname:      hostname,
		PackageID:     packageID,
		PricingID:     pricingID,
		OrderFormID:   11,
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

	var orderResp OrderResponse
	unwrapped := unwrapData(respBody)
	if err := json.Unmarshal(unwrapped, &orderResp); err != nil {
		return nil, fmt.Errorf("parsing order response: %w (body: %s)", err, string(respBody))
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
			PackageID            int64 `json:"package_id"`
			AvailableConfigOpts []struct {
				Options []struct {
					OptionID int64 `json:"option_id"`
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

func (c *SHCClient) CreateSnapshot(ctx context.Context, serviceID, name string) (*SnapshotResponse, error) {
	return retryOnLockValue(ctx, func() (*SnapshotResponse, error) {
		return c.createSnapshotOnce(ctx, serviceID, name)
	})
}

func (c *SHCClient) createSnapshotOnce(ctx context.Context, serviceID, name string) (*SnapshotResponse, error) {
	path := "/vm/" + serviceID + "/snapshots"

	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, fmt.Errorf("marshaling snapshot request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("create snapshot failed (status %d): %s", statusCode, string(respBody))
	}

	var snapResp SnapshotResponse
	if err := json.Unmarshal(respBody, &snapResp); err != nil {
		return nil, fmt.Errorf("parsing snapshot response: %w (body: %s)", err, string(respBody))
	}

	return &snapResp, nil
}

func (c *SHCClient) GetSnapshots(ctx context.Context, serviceID string) ([]SnapshotResponse, error) {
	path := "/vm/" + serviceID + "/snapshots"

	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, nil
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get snapshots failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var snaps []SnapshotResponse
	if err := json.Unmarshal(unwrapped, &snaps); err == nil {
		return snaps, nil
	}

	var wrapped struct {
		Snapshots []SnapshotResponse `json:"snapshots"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Snapshots != nil {
		return wrapped.Snapshots, nil
	}

	var items struct {
		Items []SnapshotResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return items.Items, nil
	}

	return nil, fmt.Errorf("unable to parse snapshots response: %s", string(respBody))
}

func (c *SHCClient) DeleteSnapshot(ctx context.Context, serviceID, snapshotID string) error {
	return retryOnLock(ctx, func() error {
		return c.deleteSnapshotOnce(ctx, serviceID, snapshotID)
	})
}

func (c *SHCClient) deleteSnapshotOnce(ctx context.Context, serviceID, snapshotID string) error {
	path := "/vm/" + serviceID + "/snapshots/delete"

	body, err := json.Marshal(map[string]string{"snapshot_id": snapshotID})
	if err != nil {
		return fmt.Errorf("marshaling delete snapshot request: %w", err)
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
		return fmt.Errorf("delete snapshot failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) CreateBackup(ctx context.Context, serviceID, name string) (*BackupResponse, error) {
	return retryOnLockValue(ctx, func() (*BackupResponse, error) {
		return c.createBackupOnce(ctx, serviceID, name)
	})
}

func (c *SHCClient) createBackupOnce(ctx context.Context, serviceID, name string) (*BackupResponse, error) {
	path := "/vm/" + serviceID + "/backups"

	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, fmt.Errorf("marshaling backup request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("create backup failed (status %d): %s", statusCode, string(respBody))
	}

	var backupResp BackupResponse
	if err := json.Unmarshal(respBody, &backupResp); err != nil {
		return nil, fmt.Errorf("parsing backup response: %w (body: %s)", err, string(respBody))
	}

	return &backupResp, nil
}

func (c *SHCClient) GetBackups(ctx context.Context, serviceID string) ([]BackupResponse, error) {
	path := "/vm/" + serviceID + "/backups"

	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, nil
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get backups failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var backups []BackupResponse
	if err := json.Unmarshal(unwrapped, &backups); err == nil {
		return backups, nil
	}

	var wrapped struct {
		Backups []BackupResponse `json:"backups"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Backups != nil {
		return wrapped.Backups, nil
	}

	var items struct {
		Items []BackupResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return items.Items, nil
	}

	return nil, fmt.Errorf("unable to parse backups response: %s", string(respBody))
}

func (c *SHCClient) DeleteBackup(ctx context.Context, serviceID, backupID string) error {
	return retryOnLock(ctx, func() error {
		return c.deleteBackupOnce(ctx, serviceID, backupID)
	})
}

func (c *SHCClient) deleteBackupOnce(ctx context.Context, serviceID, backupID string) error {
	path := "/vm/" + serviceID + "/backups/delete"

	body, err := json.Marshal(map[string]string{"backup_id": backupID})
	if err != nil {
		return fmt.Errorf("marshaling delete backup request: %w", err)
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
		return fmt.Errorf("delete backup failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

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

// CheckCredit verifies that the account has at least minRequired USD of
// available credit before placing an order. It fails open: if the balance
// endpoint is unreachable or the response cannot be parsed, it returns nil so
// that ordering is not blocked by a transient billing-API outage.
func (c *SHCClient) CheckCredit(ctx context.Context, minRequired float64) error {
	bal, err := c.GetBalance(ctx)
	if err != nil {
		return nil
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
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, "/ordering/catalog", nil, "")
	if err != nil || statusCode >= 400 {
		return 0
	}

	unwrapped := unwrapData(respBody)
	var catalogResp struct {
		Items []CatalogPackageResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &catalogResp); err != nil {
		return 0
	}

	for _, pkg := range catalogResp.Items {
		if pkg.PackageID == packageID {
			for _, p := range pkg.Pricing {
				if p.Period == "day" {
					f, _ := strconv.ParseFloat(p.Price.String(), 64)
					return f
				}
			}
		}
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
			Total flexibleString `json:"total"`
			Paid  flexibleString `json:"paid"`
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

type CatalogPricingResponse struct {
	Period    string         `json:"period"`
	Price     flexibleString `json:"price"`
	PricingID int64          `json:"pricing_id"`
}

type CatalogPackageResponse struct {
	PackageID int64                    `json:"package_id"`
	Name      string                   `json:"name"`
	CPU       int64                    `json:"cpu"`
	MemoryMB  int64                    `json:"memory_mb"`
	DiskGB    int64                    `json:"disk_gb"`
	Pricing   []CatalogPricingResponse `json:"pricing"`
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

func (c *SHCClient) CreateFirewallRule(ctx context.Context, serviceID string, body []byte) (*FirewallRuleResponse, error) {
	path := "/vm/" + serviceID + "/firewall/rules"

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("create firewall rule failed (status %d): %s", statusCode, string(respBody))
	}

	var ruleResp FirewallRuleResponse
	unwrapped := unwrapData(respBody)
	if err := json.Unmarshal(unwrapped, &ruleResp); err != nil {
		return nil, fmt.Errorf("parsing firewall rule response: %w (body: %s)", err, string(respBody))
	}

	return &ruleResp, nil
}

func (c *SHCClient) GetFirewall(ctx context.Context, serviceID string) (*FirewallResponse, error) {
	path := "/vm/" + serviceID + "/firewall"

	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, nil
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get firewall failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var fwResp FirewallResponse
	if err := json.Unmarshal(unwrapped, &fwResp); err == nil && fwResp.Rules != nil {
		return &fwResp, nil
	}

	var rules []FirewallRuleResponse
	if err := json.Unmarshal(unwrapped, &rules); err == nil && rules != nil {
		return &FirewallResponse{Rules: rules}, nil
	}

	var wrapped struct {
		Rules []FirewallRuleResponse `json:"rules"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Rules != nil {
		return &FirewallResponse{Rules: wrapped.Rules}, nil
	}

	var items struct {
		Items []FirewallRuleResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return &FirewallResponse{Rules: items.Items}, nil
	}

	return &FirewallResponse{}, nil
}

func (c *SHCClient) DeleteFirewallRule(ctx context.Context, serviceID string, position int64) error {
	path := "/vm/" + serviceID + "/firewall/rules/" + fmt.Sprintf("%d", position)

	statusCode, respBody, err := c.doRequest(ctx, http.MethodDelete, path, nil, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodDelete, path, nil, respBody)
		return err
	}

	if statusCode == http.StatusNotFound {
		return nil
	}

	if statusCode >= 400 {
		return fmt.Errorf("delete firewall rule failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) SetReverseDNS(ctx context.Context, serviceID, ip, hostname string) (*RDNSResponse, error) {
	path := "/vm/" + serviceID + "/rdns"

	body, err := json.Marshal(map[string]string{"ip": ip, "hostname": hostname})
	if err != nil {
		return nil, fmt.Errorf("marshaling rDNS request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("set rDNS failed (status %d): %s", statusCode, string(respBody))
	}

	var rdnsResp RDNSResponse
	unwrapped := unwrapData(respBody)
	if err := json.Unmarshal(unwrapped, &rdnsResp); err != nil {
		return nil, fmt.Errorf("parsing rDNS response: %w (body: %s)", err, string(respBody))
	}

	return &rdnsResp, nil
}

func (c *SHCClient) GetReverseDNS(ctx context.Context, serviceID string) ([]RDNSRecord, error) {
	path := "/vm/" + serviceID + "/rdns"

	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, nil
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get rDNS failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var records []RDNSRecord
	if err := json.Unmarshal(unwrapped, &records); err == nil && records != nil {
		return records, nil
	}

	var wrapped struct {
		Records []RDNSRecord `json:"records"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Records != nil {
		return wrapped.Records, nil
	}

	var items struct {
		Items []RDNSRecord `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return items.Items, nil
	}

	return nil, fmt.Errorf("unable to parse rDNS response: %s", string(respBody))
}

func (c *SHCClient) ClearReverseDNS(ctx context.Context, serviceID, ip string) error {
	path := "/vm/" + serviceID + "/rdns"

	body, err := json.Marshal(map[string]string{"ip": ip})
	if err != nil {
		return fmt.Errorf("marshaling clear rDNS request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodDelete, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodDelete, path, body, respBody)
		return err
	}

	if statusCode == http.StatusNotFound {
		return nil
	}

	if statusCode >= 400 {
		return fmt.Errorf("clear rDNS failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
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

func (c *SHCClient) RestoreSnapshot(ctx context.Context, serviceID, snapshotID string) error {
	path := "/vm/" + serviceID + "/snapshots/restore"

	body, err := json.Marshal(map[string]string{"snapshot_id": snapshotID})
	if err != nil {
		return fmt.Errorf("marshaling restore snapshot request: %w", err)
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
		return fmt.Errorf("restore snapshot failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) RestoreBackup(ctx context.Context, serviceID, backupID string) error {
	path := "/vm/" + serviceID + "/backups/restore"

	body, err := json.Marshal(map[string]string{"backup_id": backupID})
	if err != nil {
		return fmt.Errorf("marshaling restore backup request: %w", err)
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
		return fmt.Errorf("restore backup failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) UpgradeVM(ctx context.Context, serviceID string, pricingRef int64) error {
	path := "/vm/" + serviceID + "/upgrade"

	body, _ := json.Marshal(map[string]interface{}{
		"pricing_ref":     pricingRef,
		"idempotency_key": fmt.Sprintf("tf-upgrade-%d", time.Now().UnixNano()),
	})

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPatch, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPatch, path, body, respBody)
		return err
	}

	if statusCode >= 400 {
		return fmt.Errorf("upgrade failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
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

// ── VM term + addons (v2.5.0) ──────────────────────────────

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

// ── Orders (v2.5.0) ────────────────────────────────────────

func (c *SHCClient) ListOrders(ctx context.Context) (json.RawMessage, error) {
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, "/orders", nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("list orders failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) GetOrder(ctx context.Context, orderID string) (json.RawMessage, error) {
	path := "/orders/" + orderID
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("get order failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) CancelPendingOrder(ctx context.Context, orderID string) error {
	path := "/orders/" + orderID + "/cancel"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, nil, "")
	if err != nil {
		return err
	}
	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPost, path, nil, respBody)
		return err
	}
	if statusCode >= 400 {
		return fmt.Errorf("cancel order failed (status %d): %s", statusCode, string(respBody))
	}
	return nil
}
