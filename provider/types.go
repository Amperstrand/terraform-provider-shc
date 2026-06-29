package provider

import (
	"encoding/json"
	"fmt"
)

type flexibleString string

func (f *flexibleString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = flexibleString(s)
		return nil
	}

	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = flexibleString(n.String())
		return nil
	}

	return fmt.Errorf("flexibleString: cannot unmarshal %s into string", string(data))
}

func (f flexibleString) String() string {
	return string(f)
}

func (f flexibleString) GoString() string {
	return fmt.Sprintf("flexibleString(%q)", string(f))
}

type OrderRequest struct {
	Hostname    string `json:"hostname"`
	PackageID   int64  `json:"package_id"`
	PricingID   int64  `json:"pricing_id"`
	OrderFormID int64  `json:"order_form_id"`
}

type OrderResponse struct {
	ServiceIDs []flexibleString `json:"service_ids"`
	ServiceID  flexibleString   `json:"service_id"`
	ID         flexibleString   `json:"id"`
}

func (o *OrderResponse) ResolveServiceID() string {
	if len(o.ServiceIDs) > 0 && o.ServiceIDs[0].String() != "" {
		return o.ServiceIDs[0].String()
	}
	if o.ServiceID.String() != "" {
		return o.ServiceID.String()
	}
	if o.ID.String() != "" {
		return o.ID.String()
	}
	return ""
}

type dataWrapper struct {
	Data json.RawMessage `json:"data"`
}

type VMResponse struct {
	ServiceID         flexibleString `json:"service_id"`
	IPs               []vmIP         `json:"ips"`
	Hostname          string         `json:"hostname"`
	OSUser            string         `json:"os_user"`
	Status            string         `json:"service_status"`
	ProvisioningState string         `json:"provisioning_state"`
}

type vmIP struct {
	IP string `json:"ip"`
}

func (v *VMResponse) GetIP() string {
	if len(v.IPs) > 0 {
		return v.IPs[0].IP
	}
	return ""
}

type CancelRequest struct {
	Immediate bool `json:"immediate,omitempty"`
}

type CancelResponse struct {
	ConfirmationID string `json:"confirmation_id"`
	Message        string `json:"message"`
	Success        bool   `json:"success"`
}

type SnapshotResponse struct {
	ID     flexibleString `json:"id"`
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Date   string         `json:"date"`
}

type BackupResponse struct {
	ID     flexibleString `json:"id"`
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Date   string         `json:"date"`
}

type BalanceResponse struct {
	Balance  flexibleString `json:"balance"`
	Credit   flexibleString `json:"credit"`
	Currency string         `json:"currency"`
}

type confirmationResponse struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
	Confirmation struct {
		StructuredContent struct {
			ConfirmationID string `json:"confirmation_id"`
		} `json:"structuredContent"`
	} `json:"confirmation"`
}

func (c confirmationResponse) GetConfirmationID() string {
	return c.Confirmation.StructuredContent.ConfirmationID
}
