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
)

const defaultBaseURL = "https://blesta.sovereignhybridcompute.com/user-api/v2"

var ErrVMNotFound = errors.New("vm not found")

type SHCClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewSHCClient(apiKey, endpoint string) *SHCClient {
	if endpoint == "" {
		endpoint = defaultBaseURL
	}

	return &SHCClient{
		baseURL: strings.TrimRight(endpoint, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
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

func (c *SHCClient) SubmitOrder(ctx context.Context, hostname string, packageID, pricingID int64) (*OrderResponse, error) {
	orderReq := OrderRequest{
		Hostname:    hostname,
		PackageID:   packageID,
		PricingID:   pricingID,
		OrderFormID: 11,
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

	var snaps []SnapshotResponse
	if err := json.Unmarshal(respBody, &snaps); err == nil {
		return snaps, nil
	}

	var wrapped struct {
		Snapshots []SnapshotResponse `json:"snapshots"`
	}
	if err := json.Unmarshal(respBody, &wrapped); err == nil {
		return wrapped.Snapshots, nil
	}

	return nil, fmt.Errorf("unable to parse snapshots response: %s", string(respBody))
}

func (c *SHCClient) DeleteSnapshot(ctx context.Context, serviceID, snapshotID string) error {
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

	var backups []BackupResponse
	if err := json.Unmarshal(respBody, &backups); err == nil {
		return backups, nil
	}

	var wrapped struct {
		Backups []BackupResponse `json:"backups"`
	}
	if err := json.Unmarshal(respBody, &wrapped); err == nil {
		return wrapped.Backups, nil
	}

	return nil, fmt.Errorf("unable to parse backups response: %s", string(respBody))
}

func (c *SHCClient) DeleteBackup(ctx context.Context, serviceID, backupID string) error {
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
