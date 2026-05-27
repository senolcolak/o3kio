# Security Policy

## Project Status

O3K is in **alpha**. It has not been audited and is not recommended for
production deployments. The threat model below describes what we do and
do not protect against today.

## Reporting a Vulnerability

**Please do not file public GitHub issues for security vulnerabilities.**

Email reports to the maintainers privately, or use GitHub's
"Report a vulnerability" feature on this repository
(`Security` tab → `Report a vulnerability`).

When reporting, include:
- Affected version or commit SHA
- A description of the issue and its impact
- Reproduction steps or a proof-of-concept
- Whether the issue has been disclosed elsewhere

We will acknowledge receipt within **5 business days** and aim to
provide a remediation plan within **30 days** for confirmed
vulnerabilities. We do not currently run a bounty program.

## Supported Versions

Only the `main` branch receives security fixes during the alpha phase.
Tagged releases are snapshots, not supported branches. When O3K reaches
a stable release, this policy will be revised.

## Threat Model

### What O3K Defends Against

- **Unauthenticated access to API surface**: All services except the
  EC2-style metadata endpoint require a valid Keystone JWT.
- **Cross-project resource access**: Resources are scoped to the
  `project_id` carried in the JWT. Database queries filter by project.
- **Token forgery**: Tokens are HMAC-SHA256 signed by Keystone using
  the configured `jwt_secret`.
- **SSH key DoS via pathological inputs**: We track upstream CVEs in
  `golang.org/x/crypto` and gate CI on `govulncheck`.

### What O3K Does Not Defend Against (Yet)

- **Multi-tenant isolation at the hypervisor level**: Stub mode shares
  process state. Real mode relies on libvirt + namespaces but has not
  been audited for tenant escape.
- **Network-level isolation in stub mode**: Stub mode does not create
  network namespaces or iptables rules.
- **Default credentials**: The seed migration creates `admin`/`secret`.
  Operators must change this before any non-development use.
- **Secrets at rest**: JWT secret and database credentials are read
  from config / env vars; we do not yet integrate with a KMS.
- **Rate limiting and abuse**: There is no built-in request quota or
  rate limiter. Front O3K with a reverse proxy in any shared setting.
- **Audit logging**: Authentication events are logged; resource-level
  audit trails are incomplete.

## Security-Relevant Configuration

The following settings change the security posture and **must** be
reviewed before any deployment beyond local development:

| Setting | Risk if left at default |
|---|---|
| `keystone.jwt_secret` | Forgeable tokens — anyone with the default secret can impersonate any user |
| Seed credentials (`admin`/`secret`) | Trivially guessable admin access |
| `database.url` (with embedded password) | Credential leak via process listing or config dump |
| TLS termination | O3K speaks plain HTTP by default; terminate TLS at a reverse proxy |

## Dependency Vulnerabilities

CI runs `govulncheck` on every push and pull request. A non-empty
result fails the build. Known CVEs in transitive dependencies are
addressed by bumping the offending module; see commit history for
examples.

## Cryptography

- **Token signing**: HMAC-SHA256 (stateless tokens, no server-side
  storage)
- **Password hashing**: bcrypt (cost 10) for user passwords in the
  database
- **TLS**: not terminated by O3K; deploy behind a TLS-terminating
  proxy

## Disclosure Policy

We follow coordinated disclosure. Please give us a reasonable window
to ship a fix before publishing details. We will credit reporters in
the release notes unless asked otherwise.
