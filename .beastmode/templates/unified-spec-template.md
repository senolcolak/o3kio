# Feature Specification: [FEATURE NAME]

<!-- 
  TEMPLATE GUIDE:
  This template combines Spec-Kit's specification rigor with Beastmode's
  knowledge tracking. Fill out all mandatory sections (marked with *).
  
  REMEMBER:
  - ✅ Focus on WHAT and WHY (not HOW)
  - ❌ No implementation details
  - ❌ No tech stack decisions
  - ✅ Mark uncertainties with [NEEDS CLARIFICATION: question]
-->

**Feature Number**: NNN  <!-- Auto-generated: 001, 002, etc. -->
**Feature Branch**: `NNN-[short-name]`  <!-- Auto-generated -->
**Beastmode State**: `.beastmode/state/design/NNN-[feature].md`
**Status**: Draft → Specified → Planned → Implemented → Released
**Created**: [DATE]
**Input**: User description: "$ARGUMENTS"

---

## Beastmode Metadata

### Phase Information
- **Current Phase**: design
- **Next Phase**: plan
- **Worktree**: `.beastmode/worktrees/NNN-[feature]/`
- **Last Retro**: [DATE or None]

### Gray Areas (Design Decisions)
<!-- Document key decisions made during design phase -->
| Decision Area | Decision Made | Rationale |
|--------------|---------------|-----------|
| [e.g., Auth method] | [e.g., JWT] | [e.g., Stateless API requirement] |
| [NEEDS CLARIFICATION: ...] | | |

### Deferred Ideas
<!-- Features intentionally out of scope -->
- [Idea]: [Why deferred / When might revisit]

---

## User Scenarios & Testing *(mandatory)*

<!--
  User stories should be PRIORITIZED as user journeys ordered by importance.
  Each must be INDEPENDENTLY TESTABLE - implement just ONE and you have viable MVP.
  
  Priority levels: P1 (Critical), P2 (Important), P3 (Nice to have)
-->

### User Story 1 - [Brief Title] (Priority: P1)

[Describe this user journey in plain language - what user wants to accomplish]

**Why this priority**: [Explain the value and why this is P1]

**Independent Test**: [Describe how to test this standalone]

**Acceptance Scenarios**:

1. **Given** [initial state], **When** [action], **Then** [expected outcome]
2. **Given** [initial state], **When** [action], **Then** [expected outcome]

**Constitution Check**:
- [ ] Library-First: Can this be a standalone library?
- [ ] CLI Interface: Can this be tested via CLI?
- [ ] Test-First: Are scenarios testable before implementation?

---

### User Story 2 - [Brief Title] (Priority: P2)

[Description]

**Why this priority**: [Value proposition]

**Independent Test**: [Test approach]

**Acceptance Scenarios**:

1. **Given** [state], **When** [action], **Then** [outcome]

---

### User Story 3 - [Brief Title] (Priority: P3)

[Description]

**Why this priority**: [Value proposition]

**Independent Test**: [Test approach]

**Acceptance Scenarios**:

1. **Given** [state], **When** [action], **Then** [outcome]

---

[Add more user stories as needed]

### Edge Cases *(mandatory)*

<!-- Consider boundary conditions, error scenarios, limits -->

- What happens when [boundary condition, e.g., "user enters 1000 characters"]?
- How does system handle [error scenario, e.g., "database is unavailable"]?
- What if [race condition, e.g., "two users edit same item simultaneously"]?
- What happens at [scale limit, e.g., "1 million records"]?
- How is [security scenario, e.g., "SQL injection attempt"] handled?

---

## Requirements *(mandatory)*

<!--
  Mark ALL ambiguities with [NEEDS CLARIFICATION: specific question]
  Don't guess - if prompt doesn't specify, mark it.
-->

### Functional Requirements

- **FR-001**: System MUST [specific capability]
- **FR-002**: System MUST [specific capability]
- **FR-003**: Users MUST be able to [key interaction]
- **FR-004**: System MUST [data requirement]
- **FR-005**: System MUST [behavior requirement]

*Example of marking unclear requirements:*

- **FR-006**: System MUST authenticate users via [NEEDS CLARIFICATION: auth method not specified - email/password, SSO, OAuth?]
- **FR-007**: System MUST retain user data for [NEEDS CLARIFICATION: retention period not specified]

### Non-Functional Requirements

- **NFR-001 (Performance)**: System MUST [performance criteria, e.g., "respond within 200ms for 95th percentile"]
- **NFR-002 (Scalability)**: System MUST [scalability criteria, e.g., "handle 10,000 concurrent users"]
- **NFR-003 (Security)**: System MUST [security criteria, e.g., "encrypt data at rest and in transit"]
- **NFR-004 (Reliability)**: System MUST [reliability criteria, e.g., "99.9% uptime"]

### Constraints

- **CON-001**: [Business constraint, e.g., "Must integrate with existing auth system"]
- **CON-002**: [Technical constraint, e.g., "Must support IE11"]
- **CON-003**: [Regulatory constraint, e.g., "Must comply with GDPR"]

---

## Key Entities *(include if feature involves data)*

<!--
  Define domain entities WITHOUT implementation details.
  No database schemas, no class definitions - just concepts.
-->

- **[Entity 1]**: [What it represents, key attributes conceptually]
  - [Attribute 1]: [Type/description]
  - [Attribute 2]: [Type/description]
  - Relationships: [Related entities]
  
- **[Entity 2]**: [Description]
  - [Attributes...]

---

## Research Notes *(optional)*

<!--
  Links to research.md if exists.
  Key findings that influenced specification.
-->

**Research Document**: [Link to specs/NNN-feature/research.md or None]

**Key Findings**:
- [Finding 1]: [Summary]
- [Finding 2]: [Summary]

**Decisions Based on Research**:
- Chose [Option A] over [Option B] because [rationale]

---

## Beastmode Retro Notes

<!-- Populated after design phase retro -->

### Context Reconciliation
- **L2 Updates**: [Any context/design/*.md files updated]
- **New Patterns**: [New patterns identified]

### Meta Learnings
- **SOPs Created**: [Any new SOPs]
- **Overrides**: [Project-specific rules discovered]
- **Learnings**: [Session insights]

---

## Checklist *(mandatory - before marking Specified)*

### Completeness
- [ ] No [NEEDS CLARIFICATION] markers remain (or explicitly deferred)
- [ ] At least one P1 user story defined
- [ ] All user stories independently testable
- [ ] Acceptance criteria are measurable (not vague)
- [ ] Edge cases considered

### Constitution Compliance
- [ ] Library-First: Can be abstracted to library
- [ ] CLI Interface: Can expose functionality via CLI
- [ ] Test-First: Scenarios defined before implementation
- [ ] Simplicity: No speculative features

### Beastmode Integration
- [ ] Gray areas documented
- [ ] Deferred ideas listed
- [ ] Feature number assigned
- [ ] Branch name determined
- [ ] Worktree path set

---

## Handoff to Planning

When this spec is complete:

```
/speckit.plan [tech-stack-description]
```

Example:
```
/speckit.plan Node.js with Express, PostgreSQL database, Redis for caching
```

---

**Spec Version**: 1.0  
**Last Updated**: [DATE]  
**Approved By**: [NAME or Pending]
