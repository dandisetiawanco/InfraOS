# Threat Model and Remaining Limitations

## Trust Boundaries

1. **Browser to Reverse Proxy / API**: Untrusted internet or local client traffic; guarded by CORS allowlist, CSRF tokens, Argon2id auth, rate limits, request size caps (1 MiB JSON), and security headers.
2. **Public Demo Sessions to Real Host**: Hard server-side separation (`is_demo` flag) preventing demo users from touching Docker sockets, local allowlisted endpoints, agent enrollment, or backup endpoints.
3. **Enrolled Agent to Control Plane**: Separate credential store (`agent_credentials`) scoped to a single `(organization_id, host_id)` pair; compromised agents can be revoked without rotating user sessions.
4. **Control Plane to External Targets**: SSRF destination validator on both service monitoring probes and alert webhooks.

## Threats Addressed

- **Broken access control / cross-tenant data access**: Mitigated via server-side `requirePerm` and mandatory `organization_id` SQL filters (verified in `TestAuthRBACAndOrganizationIsolation`).
- **SQL injection**: All queries use parameterized placeholders (`?` / `$N`).
- **SSRF against cloud metadata or internal services**: Mitigated by IP/CIDR/DNS/redirect validation (verified in `TestSSRFValidatorBlocksMetadataPrivateAndDemoLocal`).
- **Agent impersonation / token replay**: Single-use enrollment tokens cannot be reused; revoked credentials immediately fail with HTTP 401.
- **Webhook forgery / replay**: Constant-time HMAC-SHA256 verification and `UNIQUE(provider, delivery_id)` deduplication.

## Known Remaining Limitations

- **DNS rebinding window**: Although `SafeDialContext` resolves and validates IPs at dial time, production Linux deployments should additionally enforce outbound egress firewall rules around the control plane host.
- **Local SQLite secret key**: In lightweight development mode, `INFRAOS_SECRET_KEY` defaults to a local development string unless overridden; production deployments must inject a strong high-entropy key from an external secret manager.
- **No remote interactive shell**: Intentionally omitted from the initial release to reduce blast radius.
- **No MFA/WebAuthn yet**: Planned for a future hardening milestone.
