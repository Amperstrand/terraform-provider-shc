# Migrating from Hetzner Cloud to SHC with Terraform

This guide shows how to migrate Hetzner Cloud Terraform configurations to SHC using the `terraform-provider-shc` provider.

## Resource Mapping

| Hetzner Cloud | SHC | Notes |
|---------------|-----|-------|
| `hcloud_server` | `shc_vm` | VM instance |
| `hcloud_snapshot` | `shc_snapshot` | VM snapshot |
| `hcloud_firewall` | `shc_firewall_rule` | Firewall rules (one rule per resource) |
| `hcloud_ssh_key` | N/A | Pass SSH key directly to `shc_vm` |
| `hcloud_volume` | N/A | No persistent disks, use snapshots |
| `hcloud_load_balancer` | N/A | Use reverse proxy on VM (Caddy, Nginx) |
| `hcloud_floating_ip` | N/A | Each VM gets a public IP automatically |
| `hcloud_network` | N/A | No VPC networking |
| `hcloud_server_network` | N/A | No VPC networking |

## Server Type Mapping

| Hetzner Server Type | SHC Package | SHC Pricing | Price |
|---------------------|-------------|-------------|-------|
| cx11 (1C/2GB/20GB) | 23 | 55 | $7.78/mo |
| cx21 (2C/4GB/40GB) | 26 | 55 | $14.83/mo |
| cx31 (2C/8GB/80GB) | 26 | 56 | $20.83/mo |
| cx41 (4C/16GB/160GB) | 81 | 245 | ~$20/mo |
| cx51 (8C/32GB/320GB) | 82 | 249 | ~$35/mo |

Note: SHC NVMe packages include more disk than Hetzner's base plans. NVMe Starter (pkg 23) includes 8GB disk, while Hetzner cx11 includes 20GB. For more disk, upgrade to a higher SHC package.

## Before: Hetzner Server

```hcl
resource "hcloud_server" "web" {
  name        = "web-01"
  server_type = "cx21"
  image       = "ubuntu-22.04"
  location    = "nbg1"
  ssh_keys    = [hcloud_ssh_key.default.id]
  labels      = {
    env  = "production"
    role = "web"
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "hcloud_ssh_key" "default" {
  name       = "default"
  public_key = file("~/.ssh/id_rsa.pub")
}

resource "hcloud_firewall" "web" {
  name = "web-firewall"
  apply_to {
    label_selector = "role=web"
  }

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "22"
    source_ips = ["0.0.0.0/0"]
  }

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "80"
    source_ips = ["0.0.0.0/0"]
  }

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "443"
    source_ips = ["0.0.0.0/0"]
  }
}

output "server_ip" {
  value = hcloud_server.web.ipv4_address
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

**Before (Hetzner):**

```hcl
resource "hcloud_snapshot" "pre_deploy" {
  server_id = hcloud_server.web.id
  description = "pre-deploy snapshot"
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

### Locations

Hetzner has multiple data centers (fsn1, nbg1, hel1, ash, etc.). SHC operates in a single location (Katy, Texas). No `location` argument is needed.

### Images

Hetzner uses slugs like `ubuntu-22.04`. SHC uses option IDs for templates:

```hcl
# Hetzner
image = "ubuntu-22.04"

# SHC - the template is selected during ordering
# Use option 126 for Dev VPS, option 174 for NVMe/SSD/HDD
# Values include: debian13-cloud, debian12-cloud, ubuntu2404-cloud, etc.
```

The template is specified when ordering via the SHC API, but the Terraform provider does not expose this option. Templates are selected through the SHC web console or API.

### SSH Keys

Hetzner requires creating SSH key resources first. SHC accepts the public key directly as a string:

```hcl
# Hetzner
resource "hcloud_ssh_key" "default" {
  name       = "default"
  public_key = file("~/.ssh/id_rsa.pub")
}

resource "hcloud_server" "web" {
  ssh_keys = [hcloud_ssh_key.default.id]
}

# SHC
resource "shc_vm" "web" {
  ssh_key = file("~/.ssh/id_rsa.pub")
}
```

### Firewall Rules

Hetzner uses a single firewall resource with multiple rules and label selectors. SHC uses one resource per rule applied directly to a VM:

```hcl
# Hetzner - one resource, multiple rules, applied by label
resource "hcloud_firewall" "web" {
  apply_to {
    label_selector = "role=web"
  }

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "22"
  }
  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "80"
  }
}

# SHC - one resource per rule, applied to VM directly
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

### Volumes

Hetzner supports persistent volumes. SHC does not. Use snapshots for backups:

```hcl
# Hetzner
resource "hcloud_volume" "data" {
  name     = "data"
  size     = 100
  server   = hcloud_server.web.id
  location = "nbg1"
}

# SHC - use snapshots instead
resource "shc_snapshot" "data_backup" {
  service_id = shc_vm.web.service_id
  name       = "data-backup"
}
```

### Load Balancers

Hetzner has managed load balancers. SHC does not. Use a reverse proxy on a VM:

```hcl
# Install Caddy or Nginx on the VM
# Configure it as a reverse proxy for your services
```

### Networks

Hetzner supports VPC networking with `hcloud_network` and `hcloud_server_network`. SHC does not. Each VM has a single public IP with firewall rules managed at the VM level.

### Labels

Hetzner uses labels for organization. SHC stores metadata locally when using `shc-compute`, but this is not exposed in Terraform.

### Floating IPs

Hetzner supports floating IPs that can be moved between servers. SHC does not. Each VM has a fixed public IP.

### Billing

Hetzner charges hourly. SHC bills daily with a minimum charge of one day, even if you use the VM for minutes.

## Migration Checklist

Before migrating from Hetzner Cloud to SHC:

1. Identify all server types and map them to SHC packages
2. Export data from Hetzner volumes if using persistent storage
3. Replace load balancers with reverse proxy configurations
4. Update firewall rules to use `shc_firewall_rule` resources
5. Set up snapshots as replacements for volume backups
6. Remove SSH key resources and pass keys directly to VMs
7. Remove network and server network resources
8. Update CI/CD pipelines to account for daily billing minimum
9. Test SSH access and firewall rules after migration
10. Update monitoring to use SHC metrics API

## Example: Complete Migration

**Original Hetzner config:**

```hcl
resource "hcloud_server" "app" {
  name        = "app-01"
  server_type = "cx31"
  image       = "debian-11"
  location    = "fsn1"
  ssh_keys    = [hcloud_ssh_key.default.id]
  labels      = {
    env  = "production"
    role = "app"
  }
}

resource "hcloud_volume" "data" {
  name     = "app-data"
  size     = 100
  server   = hcloud_server.app.id
  location = "fsn1"
}

resource "hcloud_network" "app" {
  name     = "app-network"
  ip_range = "10.0.0.0/16"
}

resource "hcloud_server_network" "app" {
  server_id  = hcloud_server.app.id
  network_id = hcloud_network.app.id
}

resource "hcloud_load_balancer" "app" {
  name       = "app-lb"
  location   = "fsn1"
  algorithm  = "round_robin"

  target {
    type         = "server"
    server_id    = hcloud_server.app.id
    use_private_ip = true
  }

  service {
    protocol        = "https"
    listen_port     = 443
    destination_port = 80
  }
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

# Configure Caddy or Nginx as reverse proxy on the VM
# This replaces the load balancer
# Remove network resources - not supported
```

## Next Steps

- See [examples/web-server/main.tf](../examples/web-server/main.tf) for a complete working example
- See [examples/ci-runner/main.tf](../examples/ci-runner/main.tf) for a CI runner example
- Read the [main README](../README.md) for complete provider documentation
- Explore the [SHC API docs](https://blesta.sovereignhybridcompute.com/user-api/docs/)