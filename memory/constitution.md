# Project Constitution

## Core Principles

### I. Library-First

Every feature MUST begin as a standalone library.

- Libraries must be self-contained
- Independently testable
- Clear purpose - no organizational-only libraries

### II. CLI Interface

Every library exposes functionality via CLI.

**Protocol**:
- Accept text as input (stdin, arguments, or files)
- Produce text as output (stdout)
- Support JSON format for structured data
- Errors to stderr

### III. Test-First (NON-NEGOTIABLE)

TDD is mandatory.

**Cycle**:
1. Write tests
2. Get user approval
3. Confirm tests FAIL (Red)
4. Implement to make tests pass (Green)
5. Refactor

No implementation code before tests.

### IV. Integration Testing

Use realistic environments:
- Real databases over mocks
- Actual service instances over stubs
- Contract tests mandatory

### V. Observability

Everything inspectable:
- Structured logging
- Text I/O for debuggability
- Clear error messages

### VI. Versioning

Semantic versioning: MAJOR.MINOR.PATCH

### VII. Simplicity

- Maximum 3 projects for initial implementation
- YAGNI principles
- No future-proofing

### VIII. Anti-Abstraction

- Use framework features directly
- Single model representation
- No premature abstraction

### IX. Integration-First

- Contract tests before implementation
- Integration tests over unit tests
- Real dependencies preferred

---

## Governance

- Constitution supersedes all other practices
- Amendments require:
  - Explicit rationale
  - Team review
  - Backwards compatibility assessment

**Version**: 1.0 | **Ratified**: 2026-03-09
