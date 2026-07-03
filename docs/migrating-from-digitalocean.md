# Migrating from DigitalOcean to SHC with Terraform

This guide shows how to migrate DigitalOcean Terraform configurations to SHC using the `terraform-provider-shc` provider.

## Resource Mapping

| DigitalOcean | SHC | Notes |
|--------------|-----|-------|
| `digitalocean_droplet` | `shc_vm` | VM instance |
| `digitalocean_droplet_snapshot` | `shc_snapshot` | VM snapshot |
| `digitalocean_firewall` | `shc_firewall_rule` | Firewall rules (one rule per resource) |
| `digitalocean_ssh_key` | N/A | Pass SSH key directly to `shc_vm` |
| `digitalocean_floating_ip` | N/A | Each VM gets a public IP automatically |
| `digitalocean_load_balancer` | N/A | Use reverse proxy on VM (Caddy, Nginx) |
| `digitalocean_volume` | N/A | No persistent disks, use snapshots |
| `digitalocean_spaces_bucket` | N/A | SHC is compute-only, use external S3-compatible storage |

## Size Mapping

| DigitalOcean Size | SHC Package | SHC Pricing | Price |
|-------------------|-------------|-------------|-------|
| s-1vcpu-1gb | 23 | 55 | $7.78/mo |
| s-1vcpu-2gb | 23 | 56 | $11.78/mo |
| s-2vcpu-2gb | 26 | 55 | $14.83/mo |
| s-2vcpu-4gb | 26 | 56 | $20.83/mo |
| c-2vcpu-4gb | 81 | 245 | ~$20/mo |
| c-4vcpu-8gb | 82 | 249 | ~$35/mo |

## Before: DigitalOcean Droplet

```hcl
resource "digitalocean_droplet" "web" {
  name      = "web-01"
  size      = "s-2vcpu-2gb"
  image     = "ubuntu-22-04-x64"
  region    = "nyc3"
  ssh_keys  = [digitalocean_ssh_key.default.fingerprint]
  monitoring = true
  tags      = ["web", "production"]

  lifecycle {
    create_before_destroy = true
  }
}

resource "digitalocean_ssh_key" "default" {
  name       = "default"
  public_key = file("~/.ssh/id_rsa.pub")
}

resource "digitalocean_firewall" "web" {
  name = "web-firewall"

  droplet_ids = [digitalocean_droplet.web.id]

  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = ["0.0.0.0/0"]
  }

  inbound_rule {
    protocol         = "tcp"
    port_range       = "80"
    source_addresses = ["0.0.0.0/0"]
  }

  inbound_rule {
    protocol         = "tcp"
    port_range       = "443"
    source_addresses = ["0.0.0.0/0"]
  }
}

output "droplet_ip" {
  value = digitalocean_droplet.web.ipv4_address
}
```

## After: SHC VM

```hcl
terraform {
  required_providers {
    shc = {
      source = "sovereignhybridcompute/shc"
    }
  }
}

variable "shc_api_key" {
  type      = string
  sensitive = true
}

provider "shc" {
  api_key = var.shc_api_key
}

resource "shc_vm" "web" {
  hostname    = "web-01"
  package_id  = 26
  pricing_id  = 55
  ssh_key     = file("~/.ssh/id_rsa.pub")
  auto_cancel = true
}

resource "shc_firewall_rule" "allow_ssh" {
  service_id = shc_vm.web.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "22"
  source     = "0.0.0.0/0"
  name       = "allow-ssh"
}

resource "shc_firewall_rule" "allow_http" {
  service_id = shc_vm.web.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "80"
  source     = "0.0.0.0/0"
  name       = "allow-http"
}

resource "shc_firewall_rule" "allow_https" {
  service_id = shc_vm.web.service_id
  action     = "accept"
  protocol   = "tcp"
  port       = "443"
  source     = "0.0.0.0/0"
  name       = "allow-https"
}

output "vm_ip" {
  value = shc_vm.web.ip
}
```

## Finding Package IDs

Use the SHC catalog to find package and pricing IDs:

```bash
shc catalog
```

Or read the catalog via the SHC API:

```hcl
data "http" "catalog" {
  url = "https://blesta.sovereignhybridcompute.com/user-api/v2/ordering/catalog"
}

output "catalog" {
  value = jsondecode(data.http.catalog.response_body)
}
```

The catalog returns all available packages. Each package has a `package_id` and an array of `pricing` options with `pricing_id` values.

For NVMe VPS packages:

- Package 23: NVMe Starter
- Package 26: NVMe Standard
- Package 30: NVMe Performance
- Package 33: NVMe Ultra

For Dev VPS packages:

- Package 81: Dev VPS Standard
- Package 82: Dev VPS Professional
- Package 83: Dev VPS Business

## Snapshot Migration

**Before (DigitalOcean):**

```hcl
resource "digitalocean_droplet_snapshot" "pre_deploy" {
  name       = "pre-deploy"
  droplet_id = digitalocean_droplet.web.id
}
```

**After (SHC):**

```hcl
resource "shc_snapshot" "pre_deploy" {
  service_id = shc_vm.web.service_id
  name       = "pre-deploy"
}

output "snapshot_id" {
  value = shc_snapshot.pre_deploy.snapshot_id
}
```

## Key Differences

### Regions

DigitalOcean has multiple regions (nyc3, sfo2, ams3, etc.). SHC operates in a single location (Katy, Texas). No `region` argument is needed.

### Images

DigitalOcean uses slugs like `ubuntu-22-04-x64`. SHC uses option IDs for templates:

```hcl
# DigitalOcean
image = "ubuntu-22-04-x64"

# SHC - the template is selected during ordering
# Use option 126 for Dev VPS, option 174 for NVMe/SSD/HDD
# Values include: debian13-cloud, debian12-cloud, ubuntu2404-cloud, etc.
```

The template is specified when ordering via the SHC API, but the Terraform provider does not expose this option. Templates are selected through the SHC web console or API.

### SSH Keys

DigitalOcean requires creating SSH key resources first. SHC accepts the public key directly as a string:

```hcl
# DigitalOcean
resource "digitalocean_ssh_key" "default" {
  name       = "default"
  public_key = file("~/.ssh/id_rsa.pub")
}

resource "digitalocean_droplet" "web" {
  ssh_keys = [digitalocean_ssh_key.default.fingerprint]
}

# SHC
resource "shc_vm" "web" {
  ssh_key = file("~/.ssh/id_rsa.pub")
}
```

### Firewall Rules

DigitalOcean uses a single firewall resource with multiple rules. SHC uses one resource per rule:

```hcl
# DigitalOcean - one resource, multiple rules
resource "digitalocean_firewall" "web" {
  inbound_rule {
    protocol   = "tcp"
    port_range = "22"
  }
  inbound_rule {
    protocol   = "tcp"
    port_range = "80"
  }
}

# SHC - one resource per rule
resource "shc_firewall_rule" "allow_ssh" {
  service_id = shc_vm.web.service_id
  protocol   = "tcp"
  port       = "22"
}
resource "shc_firewall_rule" "allow_http" {
  service_id = shc_vm.web.service_id
  protocol   = "tcp"
  port       = "80"
}
```

### Load Balancers

DigitalOcean has managed load balancers. SHC does not. Use a reverse proxy on a VM:

```hcl
# Install Caddy on the VM via cloud-init or SSH
# Then configure it as a reverse proxy
```

### Monitoring

DigitalOcean droplets have a `monitoring` flag. SHC provides metrics via the API:

```bash
shc metrics <service_id>
shc bandwidth <service_id>
```

### Billing

DigitalOcean charges hourly. SHC bills daily with a minimum charge of one day, even if you use the VM for minutes.

### No Persistent Disks

DigitalOcean supports volumes for persistent storage. SHC does not. Use snapshots for backups:

```hcl
resource "shc_snapshot" "backup" {
  service_id = shc_vm.web.service_id
  name       = "daily-backup"
}
```

### Tags

DigitalOcean uses `tags` attributes on droplets. SHC stores metadata locally when using `shc-compute`, but this is not exposed in Terraform.

## Migration Checklist

Before migrating from DigitalOcean to SHC:

1. Identify all droplet sizes and map them to SHC packages
2. Export data from DigitalOcean volumes if using persistent storage
3. Replace load balancers with reverse proxy configurations
4. Update firewall rules to use `shc_firewall_rule` resources
5. Set up snapshots as replacements for volume backups
6. Remove SSH key resources and pass keys directly to VMs
7. Update CI/CD pipelines to account for hourly proration billing
8. Test SSH access and firewall rules after migration
9. Update monitoring to use SHC metrics API

## Example: Complete Migration

**Original DigitalOcean config:**

```hcl
resource "digitalocean_droplet" "app" {
  name      = "app-01"
  size      = "s-2vcpu-4gb"
  image     = "debian-11-x64"
  region    = "nyc3"
  ssh_keys  = [digitalocean_ssh_key.default.fingerprint]
  monitoring = true
}

resource "digitalocean_volume" "data" {
  name      = "app-data"
  region    = "nyc3"
  size      = 100
  droplet_id = digitalocean_droplet.app.id
}

resource "digitalocean_load_balancer" "app" {
  name   = "app-lb"
  region = "nyc3"

  forwarding_rule {
    entry_port     = 443
    entry_protocol = "https"

    target_port     = 80
    target_protocol = "http"
  }

  droplet_ids = [digitalocean_droplet.app.id]
}
```

**Migrated SHC config:**

```hcl
resource "shc_vm" "app" {
  hostname    = "app-01"
  package_id  = 26
  pricing_id  = 56
  ssh_key     = file("~/.ssh/id_rsa.pub")
  auto_cancel = true
}

resource "shc_snapshot" "data_backup" {
  service_id = shc_vm.app.service_id
  name       = "data-backup"
}

# Configure Caddy as reverse proxy via cloud-init or SSH
# This replaces the load balancer
```

## Next Steps

- See [examples/web-server/main.tf](../examples/web-server/main.tf) for a complete working example
- See [examples/ci-runner/main.tf](../examples/ci-runner/main.tf) for a CI runner example
- Read the [main README](../README.md) for complete provider documentation
- Explore the [SHC API docs](https://blesta.sovereignhybridcompute.com/user-api/docs/)