package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// APIKeyResponse is the payload of POST /account/api-keys. The raw key
// string is returned exactly once at mint time — the API never shows it
// again (revokeApiKey is Basic+OTP-only, so treat minted keys as
// single-shot secrets: hand them to ephemeral resources, never to state).
type APIKeyResponse struct {
	ID        flexibleString `json:"id"`
	Label     string         `json:"label"`
	Scope     string         `json:"scope"`
	Key       string         `json:"key"`
	APIKey    string         `json:"api_key"`
	ExpiresAt string         `json:"expires_at"`
}

// Secret returns the raw key from whichever field the API populated.
func (k *APIKeyResponse) Secret() string {
	if k.Key != "" {
		return k.Key
	}
	return k.APIKey
}

// CreateAPIKey mints a new API key. Identity-class operation: requires
// SetBasicAuth beforehand (Bearer keys are forbidden on this endpoint —
// verified live 2026-08-27).
func (c *SHCClient) CreateAPIKey(ctx context.Context, label, scope string, expiresInDays int) (*APIKeyResponse, error) {
	if !c.UsingBasicAuth() {
		return nil, fmt.Errorf("minting API keys requires Basic auth (SetBasicAuth / provider account_username+account_password); Bearer keys are forbidden on POST /account/api-keys")
	}
	body := map[string]any{"label": label}
	if scope != "" {
		body["scope"] = scope
	}
	if expiresInDays > 0 {
		body["expires_in_days"] = expiresInDays
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling api key request: %w", err)
	}
	status, respBody, err := c.doRequest(ctx, http.MethodPost, "/account/api-keys", raw, "")
	if err != nil {
		return nil, fmt.Errorf("creating api key: %w", err)
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, fmt.Errorf("creating api key: HTTP %d: %s", status, string(respBody))
	}
	var wrap struct {
		Data APIKeyResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &wrap); err != nil {
		return nil, fmt.Errorf("decoding api key response: %w", err)
	}
	key := wrap.Data
	if key.Secret() == "" {
		// some envelopes are flat
		if err := json.Unmarshal(respBody, &key); err != nil || key.Secret() == "" {
			return nil, fmt.Errorf("api key response carried no key material: %s", string(respBody))
		}
	}
	return &key, nil
}

// ListVMs returns every VM on the account (GET /vm). Backs the list
// resource for `terraform query`.
func (c *SHCClient) ListVMs(ctx context.Context) ([]VMResponse, error) {
	status, body, err := c.doRequest(ctx, http.MethodGet, "/vm", nil, "")
	if err != nil {
		return nil, fmt.Errorf("listing vms: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("listing vms: HTTP %d: %s", status, string(body))
	}
	// Live envelope (2026-09-11): {"data": {"items": [...]}} — unwrapData
	// strips "data"; the inner object carries "items". The billing list keys
	// each VM's id as "id" (the detail endpoint uses "service_id") — accept
	// both, prefer "service_id".
	inner := unwrapData(body)
	var wrapped struct {
		Items []json.RawMessage `json:"items"`
	}
	var rawItems []json.RawMessage
	if json.Unmarshal(inner, &wrapped) == nil && len(wrapped.Items) > 0 {
		rawItems = wrapped.Items
	} else if err := json.Unmarshal(inner, &rawItems); err != nil {
		return nil, fmt.Errorf("decoding vm list: %w", err)
	}

	vms := make([]VMResponse, 0, len(rawItems))
	for _, raw := range rawItems {
		var v VMResponse
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("decoding vm list item: %w", err)
		}
		if v.ServiceID == "" {
			var ids struct {
				ID flexibleString `json:"id"`
			}
			if err := json.Unmarshal(raw, &ids); err == nil && ids.ID != "" {
				v.ServiceID = ids.ID
			}
		}
		vms = append(vms, v)
	}
	return vms, nil
}
