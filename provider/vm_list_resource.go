package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// vmListResource (#33): the `terraform query` list implementation for the
// managed shc_vm resource. The framework requires the list resource's
// Metadata name to MATCH the managed resource name ("shc_vm") and every
// ListResult to carry a non-nil ResourceIdentity — which is why
// vmResource also implements ResourceWithIdentity (service_id).
var (
	_ list.ListResourceWithConfigure = (*vmListResource)(nil)
	_ resource.ResourceWithIdentity  = (*vmResource)(nil)
)

type vmListResource struct {
	client *SHCClient
}

// NewVMListResource returns a fresh shc_vm list resource.
func NewVMListResource() list.ListResource {
	return &vmListResource{}
}

func (l *vmListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (l *vmListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists every VM on the account (`terraform query`). Identity: service_id.",
	}
}

func (l *vmListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*SHCClient)
	if !ok {
		resp.Diagnostics.AddError("unexpected provider data", "expected *SHCClient")
		return
	}
	l.client = client
}

func (l *vmListResource) List(ctx context.Context, req list.ListRequest, resp *list.ListResultsStream) {
	if l.client == nil {
		resp.Results = list.ListResultsStreamDiagnostics(errorDiags("provider not configured", "missing SHC client"))
		return
	}
	vms, err := l.client.ListVMs(ctx)
	if err != nil {
		resp.Results = list.ListResultsStreamDiagnostics(errorDiags("listing VMs", err.Error()))
		return
	}

	var vmSchema schema.Schema
	if req.IncludeResource {
		var sr resource.SchemaResponse
		(&vmResource{}).Schema(ctx, resource.SchemaRequest{}, &sr)
		vmSchema = sr.Schema
	}

	results := make([]list.ListResult, 0, len(vms))
	for i := range vms {
		vm := vms[i]
		sid := string(vm.ServiceID)

		idSchema := vmIdentitySchema()
		identity := tfsdk.ResourceIdentity{
			Raw:    tftypes.NewValue(idSchema.Type().TerraformType(ctx), map[string]tftypes.Value{
				"service_id": tftypes.NewValue(tftypes.String, sid),
			}),
			Schema: idSchema,
		}

		result := list.ListResult{Identity: &identity}
		if req.IncludeResource {
			raw, diags := vmListResourceValue(ctx, &vm, &vmSchema)
			if diags.HasError() {
				resp.Results = list.ListResultsStreamDiagnostics(diags)
				return
			}
			result.Resource = &tfsdk.Resource{Raw: raw, Schema: &vmSchema}
		}
		results = append(results, result)
	}

	resp.Results = func(push func(list.ListResult) bool) {
		for _, r := range results {
			if !push(r) {
				return
			}
		}
	}
}

// vmIdentitySchema is the identity of a managed shc_vm: the service_id,
// which is the one immutable handle for a VM across its whole lifecycle.
func vmIdentitySchema() identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"service_id": identityschema.StringAttribute{
				Description: "SHC service id — the immutable VM identity.",
			},
		},
	}
}

// IdentitySchema implements resource.ResourceWithIdentity on vmResource.
func (r *vmResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = vmIdentitySchema()
}

// vmListResourceValue builds the tftypes object for one listed VM against
// the managed resource schema: every attribute gets a value; state-known
// fields carry real data, the rest are null.
func vmListResourceValue(ctx context.Context, vm *VMResponse, vmSchema *schema.Schema) (tftypes.Value, diag.Diagnostics) {
	objType := vmSchema.Type().TerraformType(ctx)
	attrTypes := objType.(tftypes.Object).AttributeTypes

	values := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		values[name] = tftypes.NewValue(at, nil)
	}
	setString := func(name, v string) {
		if at, ok := attrTypes[name]; ok && at.Equal(tftypes.String) {
			values[name] = tftypes.NewValue(tftypes.String, v)
		}
	}
	sid := string(vm.ServiceID)
	setString("id", sid)
	setString("service_id", sid)
	setString("hostname", vm.Hostname)
	setString("ip", vm.GetIP())
	setString("status", vm.Status)
	setString("provisioning_state", vm.ProvisioningState)
	setString("os_user", vm.OSUser)

	return tftypes.NewValue(objType, values), nil
}

func errorDiags(summary, detail string) diag.Diagnostics {
	return diag.Diagnostics{diag.NewErrorDiagnostic(summary, detail)}
}

// compile-time: the list model helper types stay framework-typed
var _ = types.String{}
