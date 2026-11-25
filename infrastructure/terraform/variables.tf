variable "project_name" {
  description = "Resource naming prefix for the authorized InfraOS Linux environment."
  type        = string
  default     = "infraos"
}

variable "environment" {
  description = "Target deployment environment name (staging or production)."
  type        = string
  default     = "production"

  validation {
    condition     = contains(["staging", "production"], var.environment)
    error_message = "environment must be either staging or production."
  }
}

variable "location" {
  description = "Cloud datacenter location code."
  type        = string
  default     = "sin"
}

variable "server_type" {
  description = "Compute instance flavor sized for InfraOS API, web, and Postgres."
  type        = string
  default     = "cpx21"
}

variable "os_image" {
  description = "Linux base image for compute nodes."
  type        = string
  default     = "ubuntu-24.04"
}

variable "ssh_public_key" {
  description = "Authorized operator SSH public key supplied via environment or tfvars (never committed)."
  type        = string
  sensitive   = true
}

variable "admin_cidr_blocks" {
  description = "Explicit CIDR ranges allowed to reach SSH (port 22)."
  type        = list(string)
}

variable "private_network_cidr" {
  description = "Private RFC1918 CIDR range for internal control plane traffic."
  type        = string
  default     = "10.42.0.0/16"
}

variable "enable_dedicated_db_node" {
  description = "Optionally provision a separate private Linux node for PostgreSQL and backups."
  type        = bool
  default     = false
}
