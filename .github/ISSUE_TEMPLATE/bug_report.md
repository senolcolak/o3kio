---
name: Bug report
about: Report unexpected behavior, crashes, or API incompatibilities
title: "[BUG] "
labels: bug
assignees: ''
---

## Summary

A clear, one-paragraph description of the bug.

## Environment

- **O3K version / commit:** (output of `o3k --version` or `git rev-parse HEAD`)
- **Operating system:** (e.g. Ubuntu 24.04, macOS 14.5)
- **Go version:** (output of `go version`)
- **Service mode:** stub / real (which services? `nova.libvirt_mode`, `cinder.storage_mode`, …)
- **Database backend:** SQLite / PostgreSQL
- **Client:** OpenStack CLI / Terraform / Horizon / curl / other

## Reproduction

Minimal steps to reproduce. Include exact commands and config snippets.

```bash
# example
openstack server create --flavor m1.small --image cirros test-vm
```

## Expected behavior

What you expected to happen, ideally with a reference to the OpenStack API spec or a working OpenStack deployment.

## Actual behavior

What actually happened. Include full error output, response bodies, and any relevant log lines.

```
# logs / error output
```

## Configuration

Relevant `config/o3k.yaml` excerpts (redact secrets).

```yaml
# excerpt
```

## Additional context

Anything else that helps — workarounds tried, related issues, suspected component.
