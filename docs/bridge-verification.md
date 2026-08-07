# Pulumi TF Bridge — Verified E2E

**Date**: 2026-08-07
**Provider version**: v0.2.0
**Status**: ✅ Fully verified

## Summary

The Pulumi ↔ Terraform Bridge path has been verified end-to-end. A real VM was created and destroyed through `pulumi up` / `pulumi destroy`, proving the bridge works with terraform-provider-shc v0.2.0.

## Test Environment

- Pulumi CLI: v3.248.0
- Provider: terraform-provider-shc v0.2.0 (built from source)
- SDK language: Python (`pulumi_shc`)
- VM spec: NVMe Starter (nvme-1c-4gb), debian12-cloud, Katy-TX
- Hostname: test-bridge-e2e

## Results

```
pulumi up --yes
→ VM created in 50s
→ IP: 23.182.128.217
→ service_id: 2009
→ provisioning_state: provisioning (expected — see AGENTS.md Lesson #1)

pulumi destroy --yes
→ VM deleted in 5s
→ 0 VMs remaining (verified via shc list)
```

## How to Use

```bash
# 1. Build or download the provider binary
#    From v0.2.0 release: https://github.com/Amperstrand/terraform-provider-shc/releases/tag/v0.2.0
#    Or from source: go build -o terraform-provider-shc .

# 2. Ensure the binary is in your PATH
export PATH="/path/to/terraform-provider-shc:$PATH"

# 3. Create a Pulumi project
mkdir my-pulumi-project && cd my-pulumi-project
pulumi new python

# 4. Generate the SHC SDK
pulumi package add terraform-provider ./terraform-provider-shc

# 5. Configure the API key
pulumi config set shc:apiKey "shc_live_..." --secret

# 6. Write your program
cat > __main__.py << 'EOF'
import pulumi
import pulumi_shc as shc

vm = shc.Vm("my-vm",
    hostname="my-vm",
    size="nvme-1c-4gb",
    template="debian12-cloud",
    auto_cancel=True,
)

pulumi.export("ip", vm.ip)
pulumi.export("service_id", vm.service_id)
EOF

# 7. Deploy
pulumi up --yes

# 8. Destroy
pulumi destroy --yes
```

## Fixes Applied in v0.2.0 (required for bridge to work)

1. Dynamic `order_form_id` resolution (was hardcoded to 11)
2. `Idempotency-Key` header on order submissions
3. `waitForProvisioning`: checks `status == "active" && ip != ""` (not `provisioning_state == "ready"`)
4. `term` attribute: removed `Computed` flag (caused bridge schema error)
5. `package_id`/`pricing_id`: added `Computed` flag (provider resolves from `size`)
