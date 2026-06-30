# Web Server Example

This example provisions a complete web server on SHC with:

- A VM with SSH key injection
- Firewall rules to allow HTTP, HTTPS, and SSH
- A snapshot for rollback capability
- Optional Caddy reverse proxy configuration

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
- Firewall rules for ports 22 (SSH), 80 (HTTP), 443 (HTTPS)
- A snapshot named "initial-deploy"

After provisioning, you can SSH into the VM:

```bash
ssh debian@$(terraform output -raw vm_ip)
```

Install and configure Caddy for HTTPS:

```bash
sudo apt update
sudo apt install -y caddy

# Create a simple reverse proxy config
sudo tee /etc/caddy/Caddyfile << EOF
example.com {
    reverse_proxy localhost:3000
}

example.com:443 {
    reverse_proxy localhost:3000
}
EOF

sudo systemctl restart caddy
```

## Configuration

Adjust the following variables in `variables.tf` or pass them on the command line:

- `shc_api_key` (required) - Your SHC API key
- `hostname` - VM hostname (default: "web-server")
- `package_id` - SHC package ID (default: 26)
- `pricing_id` - SHC pricing ID (default: 55)
- `ssh_public_key` - Path to SSH public key (default: ~/.ssh/id_rsa.pub)
- `ssh_source_ranges` - Allowed SSH CIDR ranges (default: 0.0.0.0/0)
- `snapshot_name` - Snapshot name (default: "initial-deploy")

## Package Options

Available packages for web servers:

| Package | Pricing | vCPUs | RAM | Disk | Price |
|---------|---------|-------|-----|------|-------|
| 23 | 55 | 1 | 4 GB | 8 GB | $7.78/mo |
| 23 | 56 | 1 | 8 GB | 8 GB | $11.78/mo |
| 26 | 55 | 2 | 8 GB | 16 GB | $14.83/mo |
| 26 | 56 | 2 | 16 GB | 16 GB | $20.83/mo |
| 30 | 55 | 4 | 16 GB | 32 GB | $29.83/mo |
| 33 | 55 | 6 | 32 GB | 64 GB | $59.83/mo |

Use `shc catalog` to see all available packages.

## Outputs

- `vm_ip` - Public IP address of the VM
- `service_id` - SHC service ID for the VM
- `snapshot_id` - ID of the created snapshot

## Cleanup

```bash
terraform destroy -var="shc_api_key=$SHC_API_KEY"
```

This cancels the VM and deletes the snapshot.

## Next Steps

- Configure your web application on the VM
- Set up DNS to point your domain to the VM IP
- Configure Caddy or another reverse proxy for HTTPS
- Set up monitoring using `shc metrics <service_id>`