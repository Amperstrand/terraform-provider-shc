# CI Runner Example

This example provisions a self-hosted CI/CD runner on SHC with:

- A VM with SSH key injection
- Firewall rule to allow SSH only
- Auto-cancel enabled to prevent unexpected charges
- Optimized for cost efficiency

## Prerequisites

Install the SHC Terraform provider:

```bash
make install
```

Or manually:

```bash
go build -o terraform-provider-shc
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/sovereignhybridcompute/shc/0.1.0/$(uname -s | tr A-Z a-z)_$(uname -m)
mv terraform-provider-shc ~/.terraform.d/plugins/registry.terraform.io/sovereignhybridcompute/shc/0.1.0/$(uname -s | tr A-Z a-z)_$(uname -m)/
```

Set your SHC API key:

```bash
export SHC_API_KEY="shc_live_..."
```

Or create a `terraform.tfvars` file:

```hcl
shc_api_key = "shc_live_..."
```

## Quick Start

```bash
terraform init
terraform plan -var="shc_api_key=$SHC_API_KEY"
terraform apply -var="shc_api_key=$SHC_API_KEY"
```

## Usage

The VM will be created with:

- 2 vCPUs, 8 GB RAM, 16 GB disk (NVMe Standard, pkg 26)
- Your SSH public key injected
- Firewall rule for SSH only (port 22)
- Auto-cancel enabled to prevent renewal

After provisioning, you can SSH into the VM:

```bash
ssh debian@$(terraform output -raw vm_ip)
```

Install a CI runner:

```bash
# GitHub Actions runner
mkdir actions-runner && cd actions-runner
curl -o actions-runner-linux-x64-2.316.1.tar.gz -L https://github.com/actions/runner/releases/download/v2.316.1/actions-runner-linux-x64-2.316.1.tar.gz
echo "9b8f1405b1a6d320936dcb7d1a6a0d18e67cdd5c12c6f880bd5d2a4d7d5c3e6e  actions-runner-linux-x64-2.316.1.tar.gz" | shasum -a 256 -c
tar xzf ./actions-runner-linux-x64-2.316.1.tar.gz
./config.sh --url https://github.com/your-org/your-repo --token YOUR_TOKEN
./run.sh

# GitLab CI runner
curl -L "https://packages.gitlab.com/install/repositories/runner/gitlab-runner/script.deb.sh" | sudo bash
sudo apt-get install gitlab-runner
sudo gitlab-runner register \
  --url https://gitlab.com/ \
  --registration-token YOUR_TOKEN \
  --executor shell \
  --description "SHC CI runner"
```

## Configuration

Adjust the following variables in `variables.tf` or pass them on the command line:

- `shc_api_key` (required) - Your SHC API key
- `hostname` - VM hostname (default: "ci-runner")
- `package_id` - SHC package ID (default: 26)
- `pricing_id` - SHC pricing ID (default: 55)
- `ssh_public_key` - Path to SSH public key (default: ~/.ssh/id_rsa.pub)
- `ssh_source_ranges` - Allowed SSH CIDR ranges (default: 0.0.0.0/0)

## Package Options

Available packages for CI runners:

| Package | Pricing | vCPUs | RAM | Disk | Price |
|---------|---------|-------|-----|------|-------|
| 23 | 55 | 1 | 4 GB | 8 GB | $7.78/mo |
| 23 | 56 | 1 | 8 GB | 8 GB | $11.78/mo |
| 26 | 55 | 2 | 8 GB | 16 GB | $14.83/mo |
| 26 | 56 | 2 | 16 GB | 16 GB | $20.83/mo |
| 30 | 55 | 4 | 16 GB | 32 GB | $29.83/mo |
| 81 | 245 | 2 | 8 GB | 16 GB | ~$20/mo (Dev VPS) |
| 82 | 249 | 4 | 16 GB | 32 GB | ~$35/mo (Dev VPS) |

For CI/CD workloads, 2 vCPUs and 8+ GB RAM is recommended. Use `shc catalog` to see all available packages.

## Cost Optimization

This example uses `auto_cancel = true` to prevent unexpected charges. The VM will be scheduled for cancellation at the end of its billing term.

For even better cost control:

1. Use a small package (pkg 23 or 26)
2. Stop the runner when not needed:

```bash
terraform apply -var="power_state=stopped" -var="shc_api_key=$SHC_API_KEY"
```

3. Start it again when needed:

```bash
terraform apply -var="power_state=running" -var="shc_api_key=$SHC_API_KEY"
```

Note that SHC bills daily with a minimum charge of one day.

## Outputs

- `vm_ip` - Public IP address of the VM
- `service_id` - SHC service ID for the VM
- `ssh_command` - SSH command to connect to the CI runner

## Cleanup

```bash
terraform destroy -var="shc_api_key=$SHC_API_KEY"
```

This cancels the VM.

## Next Steps

- Install your preferred CI runner (GitHub Actions, GitLab CI, etc.)
- Configure runner labels and tags for job routing
- Set up monitoring using `shc metrics <service_id>`
- Consider using snapshots for pre-configured runner images