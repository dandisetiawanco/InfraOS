# Infrastructure as Code and Container Profiles

## Terraform (`infrastructure/terraform/`)

The Terraform configuration models an authorized Linux deployment environment using the pinned `hetznercloud/hcloud` provider (`~> 1.49`, Terraform `>= 1.6.0, < 2.0.0`):
- `versions.tf`: Explicit Terraform and provider version constraints.
- `variables.tf`: Validated inputs (`staging` or `production`), sensitive `ssh_public_key`, explicit `admin_cidr_blocks`, private CIDR `10.42.0.0/16`, and optional dedicated database node toggle (`enable_dedicated_db_node`).
- `main.tf`: Private RFC1918 network, subnet, SSH key resource, edge firewall allowing only ports 80, 443, and CIDR-restricted SSH 22, plus the reusable `modules/compute` module.
- `outputs.tf`: Control plane IPv4, private IPs, and network CIDR.
- `terraform.tfvars.example`: Non-secret example inputs.

### Security and Cost Considerations
- Provider tokens (`HCLOUD_TOKEN`) are never hardcoded and `*.tfvars` is ignored by Git.
- `terraform apply` is never executed automatically; no billable cloud resources are created.
- When Terraform is not installed on the local development workstation, formatting and validation (`terraform fmt -check` and `terraform validate`) run automatically in GitHub Actions (`.github/workflows/ci.yml`).

## Docker Compose (`docker-compose.yml`)

The optional infrastructure profile orchestrates PostgreSQL 16, Redis 7, Go API, standalone worker, Next.js web, Nginx reverse proxy, and opt-in profiles for Prometheus (`--profile observability`) and the sample workload (`--profile sample`).

Docker is never required for the lightweight macOS/Linux development profile.
