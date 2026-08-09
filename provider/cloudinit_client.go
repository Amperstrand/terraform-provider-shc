package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (c *SHCClient) ValidateCloudInit(ctx context.Context, serviceID, cloudInit string) (json.RawMessage, error) {
	path := "/virtual-machines/" + serviceID + "/cloud-init/validate"
	body, err := json.Marshal(map[string]string{"cloudInit": cloudInit})
	if err != nil {
		return nil, fmt.Errorf("marshaling cloud-init validate request: %w", err)
	}
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("validate cloud-init failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) UpdateCloudInit(ctx context.Context, serviceID, cloudInit string) (json.RawMessage, error) {
	return retryOnLockValue(ctx, func() (json.RawMessage, error) {
		return c.updateCloudInitOnce(ctx, serviceID, cloudInit)
	})
}

func (c *SHCClient) updateCloudInitOnce(ctx context.Context, serviceID, cloudInit string) (json.RawMessage, error) {
	path := "/virtual-machines/" + serviceID + "/cloud-init"
	body, err := json.Marshal(map[string]string{"cloudInit": cloudInit})
	if err != nil {
		return nil, fmt.Errorf("marshaling cloud-init update request: %w", err)
	}
	statusCode, respBody, err := c.doRequest(ctx, http.MethodPatch, path, body, "")
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPatch, path, body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("update cloud-init failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) DeleteCloudInit(ctx context.Context, serviceID string) (json.RawMessage, error) {
	return retryOnLockValue(ctx, func() (json.RawMessage, error) {
		return c.deleteCloudInitOnce(ctx, serviceID)
	})
}

func (c *SHCClient) deleteCloudInitOnce(ctx context.Context, serviceID string) (json.RawMessage, error) {
	path := "/virtual-machines/" + serviceID + "/cloud-init"
	statusCode, respBody, err := c.doRequest(ctx, http.MethodDelete, path, nil, "")
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodDelete, path, nil, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("delete cloud-init failed (status %d): %s", statusCode, string(respBody))
	}
	return unwrapData(respBody), nil
}

func (c *SHCClient) Batch(ctx context.Context, requests []BatchSubRequest) ([]BatchSubResponse, error) {
	if len(requests) > 25 {
		return nil, fmt.Errorf("batch supports at most 25 sub-requests, got %d", len(requests))
	}
	if len(requests) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(requests)
	if err != nil {
		return nil, fmt.Errorf("marshaling batch request: %w", err)
	}

	url := c.baseURL + "/batch"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating batch request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("shc-batch-%d", time.Now().UnixNano()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making batch request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading batch response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("batch failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var items []BatchSubResponse
	data := unwrapData(respBody)
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parsing batch response: %w (body: %s)", err, string(respBody))
	}
	return items, nil
}
