## Summary

One or two sentences describing what this PR does and why.

## Type

- [ ] feat — new capability
- [ ] fix — bug fix
- [ ] refactor — internal restructuring, no behavior change
- [ ] docs — documentation only
- [ ] test — adds or repairs tests
- [ ] chore — build, CI, deps, tooling

## Scope

Which services / packages are touched?

- [ ] Keystone / Nova / Neutron / Cinder / Glance / Placement / Metadata
- [ ] Hypervisor / networking / storage backends
- [ ] Database migration
- [ ] CI / build / release tooling
- [ ] Documentation

## Behavior change

If this changes any existing behavior, describe before vs after. Include the upgrade story for operators (config flag changes, env vars, breaking API responses).

## Test plan

- [ ] `go test ./...` passes locally
- [ ] `make lint` passes locally
- [ ] Affected integration tests in `test/` exercised
- [ ] Contract tests still pass (where applicable)
- [ ] Manually verified with: <!-- e.g. `openstack server create …` -->

```bash
# Commands run to validate
```

## SCS / OpenStack alignment

If this implements an OpenStack API surface or an SCS standard, link the spec and call out any deviations.

- Spec / standard:
- Conformance impact:

## Risk

What's the blast radius if this is wrong? Anything reviewers should look at extra carefully?

## Related

Closes #
Refs #
