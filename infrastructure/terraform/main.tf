# Provider authentication must be supplied externally via the HCLOUD_TOKEN environment variable.
# Do not hardcode credentials in Terraform source or commit terraform.tfvars.
provider "hcloud" {}

locals {
  prefix = "${var.project_name}-${var.environment}"
  common_labels = {
    project     = var.project_name
    environment = var.environment
    managed_by  = "terraform"
  }
}

resource "hcloud_network" "private" {
  name     = "${local.prefix}-net"
  ip_range = var.private_network_cidr
  labels   = local.common_labels
}

resource "hcloud_network_subnet" "app" {
  network_id   = hcloud_network.private.id
  type         = "cloud"
  network_zone = "ap-southeast"
  ip_range     = cidrsubnet(var.private_network_cidr, 8, 1)
}

resource "hcloud_ssh_key" "operator" {
  name       = "${local.prefix}-operator-key"
  public_key = var.ssh_public_key
  labels     = local.common_labels
}

resource "hcloud_firewall" "edge" {
  name   = "${local.prefix}-edge-fw"
  labels = local.common_labels

  rule {
    description = "Allow HTTPS ingress to Nginx reverse proxy"
    direction   = "in"
    protocol    = "tcp"
    port        = "443"
    source_ips  = ["0.0.0.0/0", "::/0"]
  }

  rule {
    description = "Allow HTTP for ACME TLS challenge and HTTPS redirect"
    direction   = "in"
    protocol    = "tcp"
    port        = "80"
    source_ips  = ["0.0.0.0/0", "::/0"]
  }

  rule {
    description = "Restrict SSH administration to explicit operator CIDRs"
    direction   = "in"
    protocol    = "tcp"
    port        = "22"
    source_ips  = var.admin_cidr_blocks
  }
}

module "control_plane" {
  source = "./modules/compute"

  name         = "${local.prefix}-control-01"
  server_type  = var.server_type
  image        = var.os_image
  location     = var.location
  ssh_key_ids  = [hcloud_ssh_key.operator.id]
  firewall_ids = [hcloud_firewall.edge.id]
  network_id   = hcloud_network.private.id
  private_ip   = cidrhost(hcloud_network_subnet.app.ip_range, 10)
  labels       = merge(local.common_labels, { role = "control-plane" })
}

module "database_node" {
  count  = var.enable_dedicated_db_node ? 1 : 0
  source = "./modules/compute"

  name         = "${local.prefix}-postgres-01"
  server_type  = var.server_type
  image        = var.os_image
  location     = var.location
  ssh_key_ids  = [hcloud_ssh_key.operator.id]
  firewall_ids = []
  network_id   = hcloud_network.private.id
  private_ip   = cidrhost(hcloud_network_subnet.app.ip_range, 20)
  public_ipv4  = false
  public_ipv6  = false
  labels       = merge(local.common_labels, { role = "database" })
}
