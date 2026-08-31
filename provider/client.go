package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"
)

const defaultBaseURL = "https://blesta.sovereignhybridcompute.com/user-api/v2"

// defaultUserAgent: placeholder until Configure injects the ldflags version.
const defaultUserAgent = "terraform-provider-shc/dev (SHC API v2.4.24)"

var ErrVMNotFound = errors.New("vm not found")

const (
	lockRetryDelay = 5 * time.Second
	lockMaxRetries = 3
)

// HTTP retry constants for transient failures (429 Too Many Requests, 503
// Service Unavailable). These status codes indicate the server has NOT
// processed the request, so retrying is safe for all HTTP methods including
// POST. Exponential backoff with ±20% jitter matches the Python client.
const (
	httpRetryBase  = 1 * time.Second
	httpRetryMax   = 30 * time.Second
	httpMaxRetries = 3
)

func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusServiceUnavailable
}

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
	baseURL             string
	apiKey              string
	userAgent           string
	httpClient          *http.Client
	costTracker         *CostTracker
	orderIdempotencyKey string
}

// SetUserAgent overrides the User-Agent header sent on every request.
// Configure calls it with the ldflags-injected provider version so the
// header always matches the build instead of a hardcoded patch number.
func (c *SHCClient) SetUserAgent(ua string) {
	c.userAgent = ua
}

// ClientOptions tunes the provider's HTTP behavior (issue #37). Zero
// values select the library defaults: 60s per-request timeout, 3 retries,
// no rate limiting.
type ClientOptions struct {
	Timeout      time.Duration // per-request; 0 = 60s default
	MaxRetries   int           // retryablehttp RetryMax; 0 = 3 (default), negative = disable
	RateLimitRPS float64       // max requests/second; 0 = unlimited
}

func NewSHCClient(apiKey, endpoint string) *SHCClient {
	return NewSHCClientWithOptions(apiKey, endpoint, ClientOptions{})
}

func NewSHCClientWithOptions(apiKey, endpoint string, opts ClientOptions) *SHCClient {
	if endpoint == "" {
		endpoint = defaultBaseURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = httpMaxRetries
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = opts.MaxRetries
	retryClient.RetryWaitMin = httpRetryBase
	retryClient.RetryWaitMax = httpRetryMax
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if err != nil {
			return true, nil
		}
		if resp != nil && (isRetryableStatus(resp.StatusCode) || resp.StatusCode >= 500) {
			return true, nil
		}
		return false, nil
	}
	retryClient.Backoff = retryablehttp.DefaultBackoff
	retryClient.HTTPClient.Timeout = opts.Timeout
	retryClient.Logger = nil

	var transport http.RoundTripper = retryClient.StandardClient().Transport
	if opts.RateLimitRPS > 0 {
		transport = &rateLimitedTransport{
			inner:   transport,
			limiter: rate.NewLimiter(rate.Limit(opts.RateLimitRPS), 1),
		}
	}

	c := &SHCClient{
		baseURL:    strings.TrimRight(endpoint, "/"),
		apiKey:     apiKey,
		userAgent:  defaultUserAgent,
		httpClient: &http.Client{Transport: transport, Timeout: opts.Timeout},
	}
	c.costTracker = NewCostTracker(c)
	return c
}

// rateLimitedTransport throttles every request to the configured rate
// before delegating; context cancellation aborts the wait.
type rateLimitedTransport struct {
	inner   http.RoundTripper
	limiter *rate.Limiter
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.inner.RoundTrip(req)
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

// Retry on 429/503 is handled by the retryablehttp transport configured in
// NewSHCClient. doRequest delegates directly to doRequestOnce.
func (c *SHCClient) doRequest(ctx context.Context, method, path string, body []byte, confirmID string) (int, []byte, error) {
	return c.doRequestOnce(ctx, method, path, body, confirmID)
}

// doRequestOnce executes a single HTTP request.
func (c *SHCClient) doRequestOnce(ctx context.Context, method, path string, body []byte, confirmID string) (int, []byte, error) {
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
	req.Header.Set("User-Agent", c.userAgent)
	if confirmID != "" {
		req.Header.Set("X-User-Api-Confirm", confirmID)
	}
	if method == http.MethodPost && path == "/ordering/submit" && c.orderIdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", c.orderIdempotencyKey)
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

func (c *SHCClient) storefrontIDsForPackage(packageID int64) (formID, moduleGroupID, packageGroupID int64, err error) {
	err = fmt.Errorf("package_id %d not found in static size map", packageID)
	for _, s := range sizeMap {
		if s.PackageID != packageID {
			continue
		}
		f, okf := lineOrderFormIDs[s.Line]
		m, okm := lineModuleGroupIDs[s.Line]
		p, okp := linePackageGroupIDs[s.Line]
		if !okf || !okm || !okp {
			return 0, 0, 0, err
		}
		return f, m, p, nil
	}
	return 0, 0, 0, err
}

type SweepVM struct {
	ServiceID string
	Hostname  string
	Status    string
}

func (c *SHCClient) ListVMsForSweep() ([]SweepVM, error) {
	statusCode, respBody, err := c.doRequest(context.Background(), http.MethodGet, "/vm", nil, "")
	if err != nil {
		return nil, fmt.Errorf("listing VMs: %w", err)
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("list VMs failed (status %d): %s", statusCode, string(respBody))
	}

	var listResp struct {
		Items []struct {
			ServiceID flexibleString `json:"service_id"`
			Hostname  string         `json:"hostname"`
			Status    string         `json:"service_status"`
		} `json:"items"`
	}
	unwrapped := unwrapData(respBody)
	if err := json.Unmarshal(unwrapped, &listResp); err != nil {
		return nil, fmt.Errorf("parsing VM list: %w", err)
	}

	var vms []SweepVM
	for _, item := range listResp.Items {
		vms = append(vms, SweepVM{
			ServiceID: item.ServiceID.String(),
			Hostname:  item.Hostname,
			Status:    item.Status,
		})
	}
	return vms, nil
}

// CheckCredit verifies that the account has at least minRequired USD of
// available credit before placing an order. It fails open: if the balance
// endpoint is unreachable or the response cannot be parsed, it returns nil so
// that ordering is not blocked by a transient billing-API outage.

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

// ── Cloud-init (v2.4.7+) ─────────────────────────────────

// ── Batch (v2.4.9+) ──────────────────────────────────────

type BatchSubRequest struct {
	ID     string `json:"id,omitempty"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type BatchSubResponse struct {
	ID     string          `json:"id,omitempty"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// ── VM term + addons (v2.4.3) ──────────────────────────────

// ── Orders (v2.4.3) ────────────────────────────────────────

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

// ── VM standby/resume (v2.4.6) ───────────────────────────

func (c *SHCClient) StandbyVM(ctx context.Context, serviceID string, keepIP bool) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/standby"
	body, _ := json.Marshal(map[string]interface{}{"keep_ip": keepIP})
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusConflict {
		return c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("standby VM failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) PreviewStandby(ctx context.Context, serviceID string) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/standby/preview"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("preview standby failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) ResumeVM(ctx context.Context, serviceID string) (json.RawMessage, error) {
	path := "/vm/" + serviceID + "/resume"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusConflict {
		return c.handleConfirmation(ctx, http.MethodPost, path, nil, respBody)
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("resume VM failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

// ── Events + agent sessions (v2.4.6) ─────────────────────

func (c *SHCClient) ListEvents(ctx context.Context) (json.RawMessage, error) {
	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, "/events", nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("list events failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}
