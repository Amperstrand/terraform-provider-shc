package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var _ basetypes.StringTypable = CaseInsensitiveStringType{}

type CaseInsensitiveStringType struct {
	basetypes.StringType
}

func (t CaseInsensitiveStringType) Equal(o attr.Type) bool {
	ot, ok := o.(CaseInsensitiveStringType)
	return ok && t.StringType.Equal(ot.StringType)
}

func (t CaseInsensitiveStringType) String() string {
	return "CaseInsensitiveStringType"
}

func (t CaseInsensitiveStringType) ValueFromString(ctx context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return CaseInsensitiveStringValue{StringValue: in}, nil
}

func (t CaseInsensitiveStringType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", attrValue)
	}
	stringValuable, _ := t.ValueFromString(ctx, stringValue)
	return stringValuable, nil
}

func (t CaseInsensitiveStringType) ValueType(ctx context.Context) attr.Value {
	return CaseInsensitiveStringValue{}
}

var _ basetypes.StringValuableWithSemanticEquals = CaseInsensitiveStringValue{}

type CaseInsensitiveStringValue struct {
	basetypes.StringValue
}

func (v CaseInsensitiveStringValue) Equal(o attr.Value) bool {
	ov, ok := o.(CaseInsensitiveStringValue)
	return ok && v.StringValue.Equal(ov.StringValue)
}

func (v CaseInsensitiveStringValue) Type(ctx context.Context) attr.Type {
	return CaseInsensitiveStringType{}
}

func (v CaseInsensitiveStringValue) StringSemanticEquals(ctx context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	ov, diags := other.ToStringValue(ctx)
	return strings.EqualFold(v.ValueString(), ov.ValueString()), diags
}

func NewCIString(s string) CaseInsensitiveStringValue {
	return CaseInsensitiveStringValue{StringValue: basetypes.NewStringValue(s)}
}

func NewCIStringNull() CaseInsensitiveStringValue {
	return CaseInsensitiveStringValue{StringValue: basetypes.NewStringNull()}
}
