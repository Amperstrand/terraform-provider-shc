package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestClientBasicAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("o6XPQHfhFRpoYo7ev:hunter2"))
		if got := r.Header.Get("Authorization"); got != want {
			t.Errorf("basic auth header: got %q want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	c := NewSHCClient("test-key", server.URL)
	if c.UsingBasicAuth() {
		t.Fatal("UsingBasicAuth should be false before SetBasicAuth")
	}
	c.SetBasicAuth("o6XPQHfhFRpoYo7ev", "hunter2")
	if !c.UsingBasicAuth() {
		t.Fatal("UsingBasicAuth should be true after SetBasicAuth")
	}
	if _, err := c.ListVMs(context.Background()); err != nil {
		t.Fatalf("ListVMs: %v", err)
	}
}

func TestClientBearerUnaffectedWithoutBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Bearer fallback, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	c := NewSHCClient("test-key", server.URL)
	if _, err := c.ListVMs(context.Background()); err != nil {
		t.Fatalf("ListVMs: %v", err)
	}
}

func TestCreateAPIKeyRequiresBasicAndMints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/api-keys" || r.Method != http.MethodPost {
			t.Errorf("expected POST /account/api-keys, got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
			t.Errorf("expected Basic auth, got %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["label"] != "ci-temp" || req["scope"] != "full" {
			t.Errorf("unexpected body: %v", req)
		}
		if d, ok := req["expires_in_days"].(float64); !ok || int(d) != 7 {
			t.Errorf("expected expires_in_days 7, got %v", req["expires_in_days"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id": 218, "label": "ci-temp", "scope": "full",
				"key": "shc_live_minted", "expires_at": "2026-09-17",
			},
		})
	}))
	defer server.Close()

	c := NewSHCClient("test-key", server.URL)
	if _, err := c.CreateAPIKey(context.Background(), "ci-temp", "full", 7); err == nil || !strings.Contains(err.Error(), "Basic auth") {
		t.Fatalf("CreateAPIKey without Basic should refuse, got: %v", err)
	}
	c.SetBasicAuth("o6XPQHfhFRpoYo7ev", "hunter2")
	key, err := c.CreateAPIKey(context.Background(), "ci-temp", "full", 7)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if key.Secret() != "shc_live_minted" || string(key.ID) != "218" {
		t.Errorf("unexpected key response: %+v", key)
	}
}

func TestListVMsParsesEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm" {
			t.Errorf("expected /vm, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"service_id": 1234, "hostname": "alpha", "service_status": "active",
					"provisioning_state": "provisioning", "os_user": "debian",
					"ips": []map[string]string{{"ip": "23.182.128.1"}},
				},
				{"service_id": 1235, "hostname": "beta", "service_status": "canceled"},
			},
		})
	}))
	defer server.Close()

	c := NewSHCClient("test-key", server.URL)
	vms, err := c.ListVMs(context.Background())
	if err != nil {
		t.Fatalf("ListVMs: %v", err)
	}
	if len(vms) != 2 || string(vms[0].ServiceID) != "1234" || vms[0].GetIP() != "23.182.128.1" {
		t.Errorf("unexpected vms: %+v", vms)
	}
}

func objValue(schemaType tftypes.Type, values map[string]tftypes.Value) tftypes.Value {
	all := map[string]tftypes.Value{}
	for name, at := range schemaType.(tftypes.Object).AttributeTypes {
		if v, ok := values[name]; ok {
			all[name] = v
		} else {
			all[name] = tftypes.NewValue(at, nil)
		}
	}
	return tftypes.NewValue(schemaType, all)
}

func TestEphemeralAPIKeySchemaAndGating(t *testing.T) {
	r := &apiKeyEphemeralResource{}

	var sr ephemeral.SchemaResponse
	r.Schema(context.Background(), ephemeral.SchemaRequest{}, &sr)
	if sr.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %+v", sr.Diagnostics)
	}

	// Open without Basic auth must fail with the actionable error.
	r.client = NewSHCClient("test-key", "http://127.0.0.1:1")
	cfg := tfsdk.Config{
		Raw: objValue(
			sr.Schema.Type().TerraformType(context.Background()),
			map[string]tftypes.Value{"label": tftypes.NewValue(tftypes.String, "ci-temp")},
		),
		Schema: &sr.Schema,
	}
	var or ephemeral.OpenResponse
	r.Open(context.Background(), ephemeral.OpenRequest{Config: cfg}, &or)
	if !or.Diagnostics.HasError() {
		t.Fatal("Open without Basic auth should error")
	}
	joined := fmt.Sprintf("%v", or.Diagnostics)
	if !strings.Contains(joined, "Basic-auth-only") || !strings.Contains(joined, "account_username") {
		t.Errorf("gating error should name the fix, got: %s", joined)
	}
}

func TestEphemeralAPIKeyOpenMints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id": 219, "label": "ci-temp", "scope": "operate",
				"key": "shc_live_ephemeral", "expires_at": "2026-09-17",
			},
		})
	}))
	defer server.Close()

	r := &apiKeyEphemeralResource{}
	var sr ephemeral.SchemaResponse
	r.Schema(context.Background(), ephemeral.SchemaRequest{}, &sr)

	client := NewSHCClient("test-key", server.URL)
	client.SetBasicAuth("o6XPQHfhFRpoYo7ev", "hunter2")
	r.client = client

	cfg := tfsdk.Config{
		Raw: objValue(
			sr.Schema.Type().TerraformType(context.Background()),
			map[string]tftypes.Value{"label": tftypes.NewValue(tftypes.String, "ci-temp")},
		),
		Schema: &sr.Schema,
	}
	var or ephemeral.OpenResponse
	r.Open(context.Background(), ephemeral.OpenRequest{Config: cfg}, &or)
	if or.Diagnostics.HasError() {
		t.Fatalf("Open diagnostics: %+v", or.Diagnostics)
	}
	var out struct {
		ID            types.String `tfsdk:"id"`
		Label         types.String `tfsdk:"label"`
		Scope         types.String `tfsdk:"scope"`
		ExpiresInDays types.Int64  `tfsdk:"expires_in_days"`
		ExpiresAt     types.String `tfsdk:"expires_at"`
		APIKey        types.String `tfsdk:"api_key"`
	}
	if diags := or.Result.Get(context.Background(), &out); diags.HasError() {
		t.Fatalf("result get: %+v", diags)
	}
	if out.APIKey.ValueString() != "shc_live_ephemeral" {
		t.Errorf("unexpected key: %q", out.APIKey.ValueString())
	}
	if out.Scope.ValueString() != "operate" {
		t.Errorf("scope should default to operate, got %q", out.Scope.ValueString())
	}
}

func TestVMRestartActionInvoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm/42/restart" || r.Method != http.MethodPatch {
			t.Errorf("expected POST /vm/42/restart, got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "ok"}})
	}))
	defer server.Close()

	a := &vmRestartAction{client: NewSHCClient("test-key", server.URL)}

	var asr action.SchemaResponse
	a.Schema(context.Background(), action.SchemaRequest{}, &asr)
	if asr.Diagnostics.HasError() {
		t.Fatalf("action schema diags: %+v", asr.Diagnostics)
	}

	cfg := tfsdk.Config{
		Raw: objValue(
			asr.Schema.Type().TerraformType(context.Background()),
			map[string]tftypes.Value{"vm_id": tftypes.NewValue(tftypes.String, "42")},
		),
		Schema: &asr.Schema,
	}
	var ir action.InvokeResponse
	a.Invoke(context.Background(), action.InvokeRequest{Config: cfg}, &ir)
	if ir.Diagnostics.HasError() {
		t.Fatalf("invoke diagnostics: %+v", ir.Diagnostics)
	}
}

func TestVMListResourceLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vm" {
			t.Errorf("expected /vm, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"service_id": 7, "hostname": "a", "service_status": "active"},
				{"service_id": 8, "hostname": "b", "service_status": "active"},
			},
		})
	}))
	defer server.Close()

	l := &vmListResource{client: NewSHCClient("test-key", server.URL)}

	var mr resource.MetadataResponse
	l.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "shc"}, &mr)
	if mr.TypeName != "shc_vm" {
		t.Fatalf("list resource name must match the managed resource, got %q", mr.TypeName)
	}

	var stream list.ListResultsStream
	l.List(context.Background(), list.ListRequest{IncludeResource: false}, &stream)
	if stream.Results == nil {
		t.Fatal("Results stream not set")
	}
	var got []string
	stream.Results(func(r list.ListResult) bool {
		var attrs map[string]tftypes.Value
		if err := r.Identity.Raw.As(&attrs); err != nil {
			t.Fatalf("identity decode: %v", err)
		}
		var sid string
		if err := attrs["service_id"].As(&sid); err != nil {
			t.Fatalf("service_id decode: %v", err)
		}
		got = append(got, sid)
		return true
	})
	if len(got) != 2 || got[0] != "7" || got[1] != "8" {
		t.Errorf("unexpected identities: %v", got)
	}
}

func TestVMResourceHasIdentitySchema(t *testing.T) {
	var ir resource.IdentitySchemaResponse
	(&vmResource{}).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &ir)
	if ir.Diagnostics.HasError() {
		t.Fatalf("identity schema diags: %+v", ir.Diagnostics)
	}
	if _, ok := ir.IdentitySchema.Attributes["service_id"]; !ok {
		t.Fatal("identity schema must carry service_id")
	}
}
