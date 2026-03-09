# Implementation Plan: [FEATURE NAME]

<!--
  TEMPLATE GUIDE:
  This plan translates the specification into executable implementation.
  It follows the Constitution's Nine Articles and Beastmode's wave-based execution.
  
  REMEMBER:
  - This is HOW, not WHAT (that was the spec)
  - Every decision links back to spec requirements
  - Must pass Phase -1 gates before proceeding
  - Test-first: Tests defined before implementation
-->

**Feature Number**: NNN
**Feature Branch**: `NNN-[short-name]`
**Spec Reference**: `specs/NNN-[feature]/spec.md`
**Research Reference**: `specs/NNN-[feature]/research.md` (if exists)
**Constitution**: `memory/constitution.md`
**Beastmode State**: `.beastmode/state/plan/NNN-[feature].md`
**Status**: Planning → Planned → Implementing → Implemented
**Created**: [DATE]

---

## Beastmode Metadata

### Phase Information
- **Previous Phase**: design (spec.md complete)
- **Current Phase**: plan
- **Next Phase**: implement
- **Worktree**: `.beastmode/worktrees/NNN-[feature]/`

### Planning Context
- **Tech Stack**: [Summary]
- **Estimated Waves**: [N]
- **Estimated Tasks**: [N]
- **Parallel-Safe Waves**: [Which ones]

### Constitution Gates Status
- **Simplicity Gate**: [Pass/Fail + justification if fail]
- **Anti-Abstraction Gate**: [Pass/Fail + justification if fail]
- **Integration-First Gate**: [Pass/Fail + justification if fail]

---

## Phase -1: Pre-Implementation Gates *(mandatory)*

<!--
  These gates enforce the Constitution's Articles.
  You MUST pass these or document justified exceptions.
-->

### Simplicity Gate (Article VII)

- [ ] Using ≤3 projects for initial implementation?
- [ ] No "future-proofing" or speculative complexity?
- [ ] Each project has clear, single responsibility?

**Complexity Tracking** (document any exceptions):
| Exception | Justification | Approval |
|-----------|---------------|----------|
| [If >3 projects, why?] | [Rationale] | [Pending/Approved] |

### Anti-Abstraction Gate (Article VIII)

- [ ] Using framework features directly (not wrapping them)?
- [ ] Single representation of models/data (no DTOs without justification)?
- [ ] No premature abstraction layers?

**Abstraction Justification** (if any wrappers/abstractions):
| Abstraction | Purpose | Trade-off |
|-------------|---------|-----------|
| [e.g., Custom ORM wrapper] | [Why needed] | [What we accept] |

### Integration-First Gate (Article IX)

- [ ] Contract tests defined before implementation?
- [ ] Using real dependencies (not mocks) where possible?
- [ ] Integration test scenarios documented in quickstart.md?

---

## Phase 0: Research Summary

<!--
  If research.md exists, summarize key findings here.
  If not needed, mark as N/A.
-->

**Research Document**: [Link or N/A]

### Technology Decisions

| Decision | Choice | Rationale | Spec Requirement |
|----------|--------|-----------|------------------|
| [e.g., Database] | [e.g., PostgreSQL] | [Why] | [FR-003: persist data] |
| [e.g., Auth] | [e.g., JWT] | [Why] | [NFR-003: security] |

### Resolved Clarifications

- **CLAR-001**: [Original question] → [Resolution]
- **CLAR-002**: [Original question] → [Resolution]

---

## Phase 1: Design & Contracts

### Data Model

**Document**: `specs/NNN-[feature]/data-model.md`

<!--
  Define entities, fields, relationships, validation rules.
  This is the source of truth for database schemas, API types, etc.
-->

#### Entity: [Entity Name]

**Purpose**: [What this entity represents]

**Fields**:
| Field | Type | Constraints | Source |
|-------|------|-------------|--------|
| [id] | UUID | Primary key | [Spec FR-001] |
| [name] | String | Max 255 chars, required | [Spec FR-002] |
| [created_at] | Timestamp | Auto-set | System |

**Relationships**:
- [Has many/many-to-many/belongs to]: [Other entity]

**Validation Rules**:
- [Rule 1]: [Description] - from [Spec requirement]

**State Transitions** (if applicable):
```
[State A] → [event] → [State B]
```

### API Contracts

**Directory**: `specs/NNN-[feature]/contracts/`

<!--
  Define all external interfaces.
  Include request/response schemas, error codes, examples.
-->

#### Contract: [Endpoint/Interface Name]

**Purpose**: [What this interface does]
**Spec Reference**: [Which user story/requirement this serves]

**Request**:
```json
{
  "field1": "type",
  "field2": "type"
}
```

**Response (Success)**:
```json
{
  "id": "uuid",
  "status": "success"
}
```

**Response (Error)**:
```json
{
  "error": "code",
  "message": "description"
}
```

**Error Codes**:
| Code | HTTP | Scenario |
|------|------|----------|
| [ERROR_001] | 400 | [When this happens] |

---

## Phase 2: Implementation Plan

<!--
  Organized into waves for Beastmode execution.
  Each wave has tasks. Mark parallel-safe waves.
-->

### Architecture Overview

```
[Component Diagram or Description]
```

**Projects** (Article VII - simplicity check):
1. **[Project 1]**: [Responsibility]
2. **[Project 2]**: [Responsibility]
3. **[Project 3]**: [Responsibility] (if needed)

### Wave 1: [Name] - Foundation

**Objective**: [What this wave achieves]
**Parallel-Safe**: [Yes/No - default No]
**Prerequisites**: [None or list]

#### Task 1.1: [Task Name]

**Spec Reference**: [Which requirement this implements]
**Files**:
- Create: `[path/to/file]`
- Modify: `[path/to/file]`

**Steps**:
1. [Action 1]
2. [Action 2]
3. [Action 3]

**Test-First** (Article III):
```
Test file: [path/to/test]
Test cases:
- [Test case 1]
- [Test case 2]
```

**Verification**:
```bash
[Command to verify]
# Expected output: [what success looks like]
```

#### Task 1.2: [Task Name]
...

### Wave 2: [Name] - Core Functionality

**Objective**: [What this wave achieves]
**Parallel-Safe**: [Yes/No]
**Prerequisites**: [Wave 1 complete]

#### Task 2.1: [Task Name]
...

### Wave 3: [Name] - Integration & Polish

**Objective**: [What this wave achieves]
**Parallel-Safe**: [Yes/No]
**Prerequisites**: [Wave 2 complete]

#### Task 3.1: [Task Name]
...

---

## Testing Strategy

### Test Pyramid

<!--
  Following Article III (Test-First) and Article IX (Integration-First)
-->

**File Creation Order**:
1. Contract tests (define interfaces)
2. Integration tests (real dependencies)
3. End-to-end tests (user scenarios)
4. Unit tests (edge cases, complex logic)

### Test Coverage

| Type | Count | Location | Priority |
|------|-------|----------|----------|
| Contract | [N] | `tests/contracts/` | P1 |
| Integration | [N] | `tests/integration/` | P1 |
| E2E | [N] | `tests/e2e/` | P2 |
| Unit | [N] | `tests/unit/` | P2 |

### Acceptance Criteria Mapping

| Spec Scenario | Test File | Test Case |
|---------------|-----------|-----------|
| [US-1, Scenario 1] | `[file]` | `[test name]` |
| [US-2, Scenario 1] | `[file]` | `[test name]` |

---

## Quickstart Validation

**Document**: `specs/NNN-[feature]/quickstart.md`

<!--
  Minimal steps to validate the feature works.
  Used by QA, demos, and validation phase.
-->

### Prerequisites
- [Requirement 1]
- [Requirement 2]

### Validation Steps

1. **[Step 1]**: [Action]
   - Expected: [Outcome]
   
2. **[Step 2]**: [Action]
   - Expected: [Outcome]

3. **[Step 3]**: [Action]
   - Expected: [Outcome]

---

## Beastmode Integration

### Wave to Task Mapping

```json
{
  "waves": {
    "1": {
      "tasks": ["1.1", "1.2"],
      "parallel_safe": false,
      "prerequisites": []
    },
    "2": {
      "tasks": ["2.1", "2.2", "2.3"],
      "parallel_safe": true,
      "prerequisites": ["1"]
    }
  }
}
```

### Gate Configuration

```yaml
# For this feature
plan-approval: [human/auto]
implement:
  architectural-deviation: [human/auto]
  test-first-enforcement: strict
```

---

## Risk & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| [e.g., Tech choice wrong] | Medium | High | [Prototyping, research] |
| [e.g., Integration complex] | High | Medium | [Early integration testing] |

---

## Beastmode Retro Notes

<!-- Populated after planning phase retro -->

### Context Reconciliation
- **L2 Updates**: [Any updates to context/plan/*.md]
- **Patterns**: [New patterns identified]

### Meta Learnings
- **SOPs**: [New procedures]
- **Overrides**: [Project-specific rules]
- **Learnings**: [Insights from planning]

---

## Checklist *(mandatory - before marking Planned)*

### Constitution Compliance
- [ ] Simplicity Gate passed (or justified)
- [ ] Anti-Abstraction Gate passed (or justified)
- [ ] Integration-First Gate passed
- [ ] Library-First: Projects structured as libraries
- [ ] CLI Interface: All libraries have CLI
- [ ] Test-First: Tests defined before implementation

### Completeness
- [ ] All spec requirements have implementation tasks
- [ ] Data model defined
- [ ] Contracts defined
- [ ] Test strategy documented
- [ ] Quickstart validation steps documented

### Beastmode Readiness
- [ ] Waves defined with clear objectives
- [ ] Tasks have verification steps
- [ ] Parallel-safe waves identified
- [ ] File dependencies mapped
- [ ] Gates configured

---

## Handoff to Implementation

When this plan is complete:

```
/speckit.tasks    # Generate task list
/speckit.implement [--parallel]   # Start implementation
```

---

**Plan Version**: 1.0  
**Last Updated**: [DATE]  
**Approved By**: [NAME or Pending]
