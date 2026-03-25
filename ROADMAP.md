# O3K Development Roadmap

**Version**: 1.1
**Last Updated**: 2026-03-19
**Status**: Active

## Executive Summary

O3K is transforming from a working proof-of-concept into a production-ready modular OpenStack alternative. This roadmap defines the path from the current monolithic implementation to a fully modular, enterprise-grade cloud infrastructure platform with **100% Terraform compatibility** as the primary success metric.

**Current State**: 5 core services in single binary (Keystone, Nova, Neutron, Cinder, Glance)
**Target State**: 8+ independent services, enhanced authentication, full Terraform/Horizon/CLI compatibility

## Vision

> **O3K: The Drop-in OpenStack Replacement**
>
> Zero-modification migration from OpenStack - users deploy existing Terraform scripts, use Horizon UI, and run OpenStack CLI commands unchanged. Modular, observable, fail-early cloud infrastructure that shows operators exactly what's happening—no hidden queues, no state mysteries.

### Core Principles

**PRIORITY ZERO**: [SPEC-000: 100% Terraform Compatibility](./specs/000-terraform-compatibility/README.md) - Non-negotiable requirement that supersedes all other work.

1. **100% Terraform Compatibility**: Existing `openstack_*` Terraform resources work unchanged
2. **100% UI/CLI Compatibility**: Horizon dashboard and OpenStack CLI indistinguishable from OpenStack
3. **Zero-Modification Migration**: Users switch endpoints, nothing else
4. **Modular Development**: Each service independently deployable
5. **Fail-Early Architecture**: Dependencies fail fast (< 1s), no queuing
6. **Observable Operations**: Real-time state visibility
7. **Library-First**: Everything starts as a library (Constitution Article I)
8. **Test-First**: TDD mandatory (Constitution Article III)

## Project Phases

```
┌─────────────────────────────────────────────────────────────────┐
│ Phase 1: Modular Transformation      │ 16 weeks │ Foundation   │
├─────────────────────────────────────────────────────────────────┤
│ Phase 2: Authentication Enhancement  │  8 weeks │ Security     │
├─────────────────────────────────────────────────────────────────┤
│ Phase 3: New Services                │ 12 weeks │ Expansion    │
├─────────────────────────────────────────────────────────────────┤
│ Phase 4: Production Hardening        │  8 weeks │ Stability    │
├─────────────────────────────────────────────────────────────────┤
│ Phase 5: Advanced Features           │ 12 weeks │ Enterprise   │
└─────────────────────────────────────────────────────────────────┘

Total Timeline: ~56 weeks (~13 months)
```

---

## Phase 1: Modular Transformation (Weeks 1-16)

**Goal**: Separate monolithic O3K into independent service modules

### Milestones

#### M1.1: Library Extraction (Weeks 1-4)

**Objective**: Convert internal services to standalone libraries

**Deliverables**:
- [ ] Extract Keystone: `pkg/keystone/` library + `cmd/o3k-keystone/` binary
- [ ] Extract Nova: `pkg/nova/` library + `cmd/o3k-nova/` binary
- [ ] Extract Neutron: `pkg/neutron/` library + `cmd/o3k-neutron/` binary
- [ ] Extract Cinder: `pkg/cinder/` library + `cmd/o3k-cinder/` binary
- [ ] Extract Glance: `pkg/glance/` library + `cmd/o3k-glance/` binary
- [ ] CLI tools: `o3k-{service}-cli` for each service

**Validation Gates**:
- Each service runs as standalone binary
- Unit tests pass for all services
- Contract tests pass with OpenStack CLI
- **Terraform provider compatibility: 100%** (all `openstack_*` resources tested)
- Documentation updated

**Risks**:
- Circular dependencies between services
- Shared database access patterns
- Configuration migration

#### M1.2: Service Decoupling (Weeks 5-8)

**Objective**: Replace internal calls with OpenStack API clients

**Deliverables**:
- [ ] Replace Nova → Neutron internal calls with Gophercloud
- [ ] Replace Nova → Cinder internal calls with Gophercloud
- [ ] Replace Nova → Glance internal calls with Gophercloud
- [ ] Service-to-service authentication via JWT tokens
- [ ] Service discovery configuration (static initially)

**Validation Gates**:
- Services communicate via HTTP only
- No direct function calls between services
- Integration tests pass with separate service processes
- **Terraform end-to-end test suite passes** (infrastructure lifecycle)
- < 5% performance overhead vs monolithic

**Risks**:
- Network latency between services
- Cascading failures without circuit breakers
- Token validation overhead

#### M1.3: Configuration Separation (Weeks 9-12)

**Objective**: Independent configuration per service

**Deliverables**:
- [ ] `/etc/o3k/keystone.yaml` - Keystone config
- [ ] `/etc/o3k/nova.yaml` - Nova config
- [ ] `/etc/o3k/neutron.yaml` - Neutron config
- [ ] `/etc/o3k/cinder.yaml` - Cinder config
- [ ] `/etc/o3k/glance.yaml` - Glance config
- [ ] Configuration validation per service
- [ ] Environment variable override support

**Validation Gates**:
- Each service starts with independent config
- Config changes don't require other services restart
- Secret management (no plaintext passwords)
- Configuration documentation complete

**Risks**:
- Configuration drift between services
- Secret synchronization
- Backward compatibility

#### M1.4: Docker Compose Deployment (Weeks 13-16)

**Objective**: Reference deployment with separate containers

**Deliverables**:
- [ ] `docker-compose.yaml` with 5 service containers
- [ ] PostgreSQL container with shared database
- [ ] Service health checks
- [ ] Automated startup ordering
- [ ] Volume mounts for persistent data
- [ ] Horizon 2025.2 container integration

**Validation Gates**:
- `docker compose up` starts all services
- Horizon dashboard works with modular services
- Full integration test suite passes
- **Terraform provider can provision complete infrastructure** (network, compute, storage)
- Documentation for deployment options
- Migration guide from monolithic

**Risks**:
- Service startup race conditions
- Database migration conflicts
- Port conflicts

### Phase 1 Success Criteria

- [x] All 5 services run independently
- [x] Services communicate via OpenStack APIs
- [x] Horizon 2025.2 dashboard fully functional
- [x] `openstack` CLI works with all services
- [x] **Terraform provider 100% compatible** (all resources: compute, network, storage, identity)
- [x] Docker Compose reference deployment
- [x] Migration guide published
- [x] Performance regression < 5%

---

## Phase 2: Authentication Enhancement (Weeks 17-24)

**Goal**: Enterprise-grade authentication mechanisms

### Milestones

#### M2.1: Application Credentials (Weeks 17-18)

**Objective**: Long-lived credentials for automation

**Deliverables**:
- [ ] Database schema (application_credentials table)
- [ ] API endpoints: POST/GET/DELETE `/v3/users/{user_id}/application_credentials`
- [ ] CLI integration: `openstack application credential create`
- [ ] Contract tests with OpenStack CLI
- [ ] Documentation and examples

**Validation Gates**:
- CI/CD pipeline uses application credentials
- Secret rotation works
- Expiration enforcement works
- Audit logging complete

#### M2.2: Token Revocation (Weeks 19-20)

**Objective**: Active token blacklist

**Deliverables**:
- [ ] Redis integration for revocation list
- [ ] Database table: `revoked_tokens`
- [ ] Enhanced `DELETE /v3/auth/tokens` endpoint
- [ ] Token validation middleware update (< 5ms overhead)
- [ ] Automatic cleanup of expired entries

**Validation Gates**:
- Revoked tokens rejected immediately
- Performance impact < 5ms per request
- Redis failover handling
- Load testing (10,000 revocations)

#### M2.3: OAuth2/OIDC (Weeks 21-22)

**Objective**: Modern authentication for web applications

**Deliverables**:
- [ ] OAuth2 provider configuration (Google, GitHub, Azure AD)
- [ ] OIDC discovery support
- [ ] Authorization code flow
- [ ] API endpoints: `/v3/auth/oauth2/*`
- [ ] State parameter for CSRF protection
- [ ] Horizon integration (OAuth login button)

**Validation Gates**:
- Login via Google works
- Login via GitHub works
- Login via Azure AD works
- Token exchange < 500ms
- Security audit passes

#### M2.4: LDAP/AD Integration (Weeks 23-24)

**Objective**: Corporate directory integration

**Deliverables**:
- [ ] LDAP bind authentication
- [ ] User/group synchronization
- [ ] Role mapping configuration
- [ ] API endpoint: `POST /v3/auth/tokens` (method: ldap)
- [ ] Active Directory test integration

**Validation Gates**:
- AD users can authenticate
- Group memberships map to roles
- User sync < 200ms per user
- Fallback to local auth if LDAP down

### Phase 2 Success Criteria

- [x] 4+ authentication methods supported
- [x] Horizon supports OAuth/SAML login
- [x] LDAP users authenticate successfully
- [x] Token revocation < 5ms overhead
- [x] Security audit passes
- [x] Documentation complete

---

## Phase 3: New Services (Weeks 25-36)

**Goal**: Implement Barbican, Designate, Octavia

### Milestones

#### M3.1: Barbican - Key Management (Weeks 25-30)

**Objective**: Encryption key and secret management

**Deliverables**:
- [ ] Library: `pkg/barbican/`
- [ ] Binary: `cmd/o3k-barbican/`
- [ ] CLI: `cmd/o3k-barbican-cli/`
- [ ] Database schema (6 tables)
- [ ] SoftHSM backend
- [ ] Cinder volume encryption integration
- [ ] API endpoints (secrets, containers, orders)

**Validation Gates**:
- python-barbicanclient works
- Cinder encrypted volumes work
- SoftHSM integration functional
- Secrets encrypted at rest
- HSM failover < 1s

**Timeline**:
- Weeks 25-26: Core secret management + database backend
- Week 27: SoftHSM integration
- Week 28: Containers & orders
- Week 29: Cinder integration
- Week 30: Testing & documentation

#### M3.2: Designate - DNS (Weeks 31-34)

**Objective**: DNS as a Service

**Deliverables**:
- [ ] Library: `pkg/designate/`
- [ ] Binary: `cmd/o3k-designate/`
- [ ] CLI: `cmd/o3k-designate-cli/`
- [ ] Database schema (8 tables)
- [ ] BIND9 backend
- [ ] Neutron floating IP auto-DNS
- [ ] API endpoints (zones, recordsets)

**Validation Gates**:
- python-designateclient works
- BIND9 backend functional
- DNS resolution works (dig/nslookup)
- Floating IP auto-DNS works
- Zone transfers work

**Timeline**:
- Week 31: Core zone management + stub backend
- Week 32: BIND9 backend
- Week 33: Record management + Neutron integration
- Week 34: Testing & documentation

#### M3.3: Octavia - Load Balancing (Weeks 35-36)

**Objective**: Load Balancing as a Service

**Deliverables**:
- [ ] Library: `pkg/octavia/`
- [ ] Binary: `cmd/o3k-octavia/`
- [ ] CLI: `cmd/o3k-octavia-cli/`
- [ ] Database schema (10 tables)
- [ ] HAProxy backend
- [ ] Namespace-based isolation
- [ ] API endpoints (loadbalancers, listeners, pools, members)

**Validation Gates**:
- python-octaviaclient works
- HAProxy backend functional
- Load balancing verified (curl tests)
- Health monitoring works
- TLS termination works (Barbican integration)

**Timeline**:
- Weeks 35-36: Core implementation (6-week full implementation in later phase)
- Initial release: Basic load balancing only
- Full features: Phase 5

### Phase 3 Success Criteria

- [x] 3 new services operational
- [x] OpenStack CLI integration complete
- [x] Horizon dashboard shows new services
- [x] **Terraform resources added**: `openstack_keymanager_secret_v1`, `openstack_dns_zone_v2`, `openstack_lb_loadbalancer_v2`
- [x] Volume encryption works (Barbican)
- [x] Floating IP DNS works (Designate)
- [x] Basic load balancing works (Octavia)
- [x] All contract tests pass

---

## Phase 4: Production Hardening (Weeks 37-44)

**Goal**: Production-ready deployment and operations

### Milestones

#### M4.1: Observability (Weeks 37-39)

**Objective**: Production monitoring and debugging

**Deliverables**:
- [ ] Prometheus metrics endpoints for all services
- [ ] Grafana dashboards (per service)
- [ ] Structured logging (JSON) standardized
- [ ] Distributed tracing (OpenTelemetry)
- [ ] Health check endpoints (`/healthz`)
- [ ] Readiness probes (`/readyz`)

**Validation Gates**:
- All services expose metrics
- Dashboards show real-time state
- Trace requests across services
- Alerts configured
- SLO/SLI definitions

#### M4.2: High Availability (Weeks 40-42)

**Objective**: Multi-instance service deployment

**Deliverables**:
- [ ] Active-active API servers (Keystone, Nova, etc.)
- [ ] Database connection pooling tuning
- [ ] Load balancer for API endpoints
- [ ] Graceful shutdown handling
- [ ] Leader election for singleton tasks

**Validation Gates**:
- 2+ API instances handle requests
- Zero downtime during rolling updates
- Database connection limits respected
- Failover < 5s

#### M4.3: Security Hardening (Weeks 43-44)

**Objective**: Production security standards

**Deliverables**:
- [ ] TLS/mTLS between services
- [ ] Certificate management (Let's Encrypt integration)
- [ ] Rate limiting per project/user
- [ ] API audit logging
- [ ] Security group defaults
- [ ] Secret encryption at rest

**Validation Gates**:
- Security audit passes
- Penetration testing complete
- OWASP Top 10 mitigations
- Compliance documentation (SOC 2, ISO 27001)

#### M4.4: CI/CD & E2E Testing (Weeks 43-44)

**Objective**: Automated testing and continuous integration

**Deliverables**:
- [ ] GitHub Actions CI pipeline (8 stages: Build → Release)
- [ ] Contract test automation (191 tests, target 90%+ passing)
- [ ] E2E test scenarios (Priority 1: VM lifecycle, volume attach, network isolation)
- [ ] Terraform stack validation tests
- [ ] Horizon UI testing (Selenium-based)
- [ ] Nightly full test suite
- [ ] Performance benchmarks automation
- [ ] Test result reporting and dashboards

**Validation Gates**:
- CI pipeline runs on all PRs
- Contract tests: 90%+ passing (current: 82%)
- 8 E2E scenarios operational
- Fast E2E suite < 5 minutes
- Full E2E suite < 15 minutes
- Zero false positives in gates
- Test coverage metrics tracked

**Reference**: See `E2E_CI_TESTING_STRATEGY.md` for detailed implementation plan

### Phase 4 Success Criteria

- [x] Prometheus metrics exported
- [x] HA deployment tested (3+ API instances)
- [x] TLS everywhere
- [x] Security audit passes
- [x] Zero downtime rolling updates
- [x] Disaster recovery procedures documented
- [x] CI/CD pipeline operational
- [x] E2E test coverage 90%+
- [x] Automated test gates enforced

---

## Phase 5: Advanced Features (Weeks 45-56)

**Goal**: Enterprise and advanced capabilities

### Milestones

#### M5.1: Octavia Full Features (Weeks 45-47)

**Deliverables**:
- [ ] L7 policies (URL routing)
- [ ] Session persistence
- [ ] SSL/SNI support
- [ ] Connection limits
- [ ] Statistics API

#### M5.2: Live Migration (Weeks 48-50)

**Deliverables**:
- [ ] Nova live migration support
- [ ] Shared storage detection
- [ ] Migration validation
- [ ] Cold migration

#### M5.3: Volume Features (Weeks 51-53)

**Deliverables**:
- [ ] Volume backup & restore
- [ ] Incremental backups
- [ ] Volume replication
- [ ] Snapshot trees

#### M5.4: Networking Advanced (Weeks 54-56)

**Deliverables**:
- [ ] IPv6 support (dual-stack)
- [ ] QoS policies
- [ ] BGP dynamic routing
- [ ] Service function chaining

### Phase 5 Success Criteria

- [x] All advanced features implemented
- [x] Enterprise customer deployments
- [x] Feature parity assessment complete
- [x] Performance benchmarks published

---

## Horizon 2025.2 Integration

**Strategy**: Use upstream Horizon unmodified

### Configuration

```yaml
# Horizon configuration for O3K
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
}

OPENSTACK_KEYSTONE_URL = "http://o3k-keystone:35357"

OPENSTACK_HOST = "o3k-keystone"
OPENSTACK_KEYSTONE_DEFAULT_ROLE = "member"

# OAuth2 support (Phase 2)
OPENSTACK_KEYSTONE_FEDERATION_MANAGEMENT = True
```

### Deployment

```yaml
# docker-compose.yaml
services:
  horizon:
    image: openstack/horizon:2025.2
    ports:
      - "80:80"
    environment:
      - OS_AUTH_URL=http://o3k-keystone:35357/v3
      - OS_PROJECT_DOMAIN_ID=default
      - OS_USER_DOMAIN_ID=default
    volumes:
      - ./horizon-config:/etc/openstack-dashboard
```

### Testing Matrix

| Feature | Phase 1 | Phase 2 | Phase 3 |
|---------|---------|---------|---------|
| Login | ✅ | ✅ (OAuth) | ✅ |
| Instance Management | ✅ | ✅ | ✅ |
| Network Topology | ✅ | ✅ | ✅ |
| Volume Management | ✅ | ✅ | ✅ (encrypted) |
| Image Management | ✅ | ✅ | ✅ |
| Security Groups | ✅ | ✅ | ✅ |
| DNS Management | ❌ | ❌ | ✅ |
| Load Balancers | ❌ | ❌ | ✅ |

---

## Success Metrics

### Technical Metrics

| Metric | Current | Phase 1 | Phase 3 | Phase 5 |
|--------|---------|---------|---------|---------|
| Services | 5 | 5 | 8 | 8+ |
| **Terraform Compatibility** | **100%** | **100%** | **100%** | **100%** |
| Horizon UI Compatibility | 100% | 100% | 100% | 100% |
| OpenStack CLI Compatibility | 100% | 100% | 100% | 100% |
| API Coverage | 104% | 104% | 110%+ | 115%+ |
| Test Coverage | 82% | 85% | 90% | 95% |
| **Contract Tests Passing** | **82%** | **85%** | **90%** | **95%** |
| **E2E Scenarios** | **0** | **3** | **8** | **15** |
| **CI Automation** | **0%** | **50%** | **100%** | **100%** |
| Response Time (p95) | 200ms | 250ms | 300ms | 250ms |
| Fail-Early Timeout | 1s | 1s | 1s | 1s |

### Operational Metrics

| Metric | Target |
|--------|--------|
| Service Startup Time | < 5s per service |
| Config Reload Time | < 1s (no restart) |
| Zero Downtime Updates | Yes (Phase 4) |
| HA Failover Time | < 5s (Phase 4) |
| Security Audit Score | 95%+ (Phase 4) |

### Adoption Metrics

| Milestone | Target Date |
|-----------|-------------|
| Alpha Release (Phase 1) | Week 16 (Q2 2026) |
| Beta Release (Phase 3) | Week 36 (Q4 2026) |
| Production Release (Phase 4) | Week 44 (Q1 2027) |
| Enterprise Ready (Phase 5) | Week 56 (Q2 2027) |

---

## Risk Management

### High Priority Risks

| Risk | Impact | Mitigation | Status |
|------|--------|------------|--------|
| **Terraform provider regression** | **Critical** | **Automated Terraform test suite, version pinning** | **Active** |
| Performance regression | High | Benchmark each phase, < 5% threshold | Active |
| Breaking API changes | Critical | Contract tests, versioning | Active |
| Security vulnerabilities | Critical | Weekly security scans, audits | Planned |
| Database migration conflicts | Medium | Independent schema prefixes | Planned |
| Service dependency failures | Medium | Fail-early strategy, timeouts | Active |

### Medium Priority Risks

| Risk | Impact | Mitigation | Status |
|------|--------|------------|--------|
| Configuration complexity | Medium | Docker Compose templates, docs | Planned |
| Horizon compatibility issues | Medium | Weekly compatibility tests | Active |
| Developer adoption | Medium | Clear docs, examples | Planned |
| Testing coverage gaps | Medium | TDD enforcement, coverage tools | Active |

---

## Resource Requirements

### Development Team

- **Phase 1**: 2-3 developers (modular transformation)
- **Phase 2**: 1-2 developers (authentication)
- **Phase 3**: 2-3 developers (new services)
- **Phase 4**: 2-3 developers (hardening)
- **Phase 5**: 1-2 developers (advanced features)

### Infrastructure

- **CI/CD**: GitHub Actions (8-stage pipeline: Build → Unit → Contract → Integration → E2E Fast → E2E Full → Deploy → Release)
- **Testing**: Docker Compose test environment, 191 contract tests, 15 E2E scenarios (staged rollout)
- **Monitoring**: Prometheus, Grafana stack
- **Documentation**: GitHub Pages, readthedocs

---

## Communication Plan

### Documentation

- [ ] Architectural Decision Records (ADRs) for major changes
- [ ] API documentation (OpenAPI specs)
- [ ] Deployment guides (Docker, Kubernetes, bare metal)
- [ ] Migration guides (monolithic → modular)
- [ ] Troubleshooting guides per service
- [ ] Testing strategy and CI/CD setup (see `E2E_CI_TESTING_STRATEGY.md`)

### Community

- [ ] Monthly progress updates (blog posts)
- [ ] Quarterly roadmap reviews
- [ ] GitHub Discussions for Q&A
- [ ] Demo videos for each milestone
- [ ] Conference talks (OpenInfra Summit)

---

## Dependencies

### External

- **Gophercloud**: OpenStack Go SDK
- **libvirt**: VM management
- **HAProxy**: Load balancing
- **BIND9**: DNS server
- **PostgreSQL**: Database
- **Redis**: Token revocation list
- **SoftHSM**: Key management

### Internal

```
Phase 1 ──┐
          ├─→ Phase 2 (requires modular Keystone)
          └─→ Phase 3 (requires all modular services)
               └─→ Phase 4 (requires all services + auth)
                    └─→ Phase 5 (requires hardened platform)
```

---

## Appendix

### Specifications

- [SPEC-000: Terraform Compatibility Framework](./specs/000-terraform-compatibility/README.md) - **PRIORITY ZERO**
- [SPEC-001: Modular Architecture](./specs/001-modular-architecture/README.md)
- [SPEC-002: Authentication Enhancement](./specs/002-authentication-enhancement/README.md)
- [SPEC-003: Barbican](./specs/003-barbican/README.md)
- [SPEC-004: Designate](./specs/004-designate/README.md)
- [SPEC-005: Octavia](./specs/005-octavia/README.md)

### Constitution Reference

- Article I: Library-First
- Article III: Test-First (TDD mandatory)
- Article IX: Integration-First

### OpenStack References

- [OpenStack API Documentation](https://docs.openstack.org/api-ref/)
- [Keystone v3 API](https://docs.openstack.org/api-ref/identity/v3/)
- [Nova v2.1 API](https://docs.openstack.org/api-ref/compute/)
- [Neutron v2.0 API](https://docs.openstack.org/api-ref/network/v2/)
- [Cinder v3 API](https://docs.openstack.org/api-ref/block-storage/v3/)
- [Glance v2 API](https://docs.openstack.org/api-ref/image/v2/)
- [Barbican v1 API](https://docs.openstack.org/api-ref/key-manager/)
- [Designate v2 API](https://docs.openstack.org/api-ref/dns/)
- [Octavia v2 API](https://docs.openstack.org/api-ref/load-balancer/v2/)

---

**Document Ownership**: O3K Core Team
**Review Cycle**: Quarterly
**Next Review**: 2026-06-09
