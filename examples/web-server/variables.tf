variable "shc_api_key" {
  type        = string
  description = "SHC API key for authentication"
  sensitive   = true
}

variable "hostname" {
  type        = string
  description = "Hostname for the web server"
  default     = "web-server"
}

variable "package_id" {
  type        = number
  description = "SHC package ID (26 = NVMe Standard, 2C/8GB/16GB)"
  default     = 26
}

variable "pricing_id" {
  type        = number
  description = "SHC pricing ID for the package"
  default     = 55
}

variable "ssh_public_key" {
  type        = string
  description = "Path to SSH public key file"
  default     = "~/.ssh/id_rsa.pub"
}

variable "ssh_source_ranges" {
  type        = list(string)
  description = "Allowed CIDR ranges for SSH access"
  default     = ["0.0.0.0/0"]
}

variable "snapshot_name" {
  type        = string
  description = "Name for the initial snapshot"
  default     = "initial-deploy"
}