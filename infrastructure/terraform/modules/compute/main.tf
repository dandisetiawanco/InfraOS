variable "name" {
  type = string
}

variable "server_type" {
  type = string
}

variable "image" {
  type = string
}

variable "location" {
  type = string
}

variable "ssh_key_ids" {
  type = list(number)
}

variable "firewall_ids" {
  type    = list(number)
  default = []
}

variable "network_id" {
  type = number
}

variable "private_ip" {
  type = string
}

variable "public_ipv4" {
  type    = bool
  default = true
}

variable "public_ipv6" {
  type    = bool
  default = true
}

variable "labels" {
  type    = map(string)
  default = {}
}

resource "hcloud_server" "node" {
  name         = var.name
  server_type  = var.server_type
  image        = var.image
  location     = var.location
  ssh_keys     = var.ssh_key_ids
  firewall_ids = var.firewall_ids
  labels       = var.labels

  public_net {
    ipv4_enabled = var.public_ipv4
    ipv6_enabled = var.public_ipv6
  }

  network {
    network_id = var.network_id
    ip         = var.private_ip
  }

  user_data = <<-EOT
    #cloud-config
    package_update: true
    packages:
      - ca-certificates
      - curl
      - ufw
    runcmd:
      - ufw default deny incoming
      - ufw default allow outgoing
      - ufw allow 22/tcp
      - ufw allow 80/tcp
      - ufw allow 443/tcp
  EOT
}

output "id" {
  value = hcloud_server.node.id
}

output "ipv4_address" {
  value = hcloud_server.node.ipv4_address
}

output "private_ip" {
  value = var.private_ip
}
