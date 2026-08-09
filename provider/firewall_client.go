package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

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
