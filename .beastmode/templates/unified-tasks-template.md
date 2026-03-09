# Tasks: [FEATURE NAME]

<!--
  TEMPLATE GUIDE:
  This file bridges Spec-Kit's task planning with Beastmode's execution.
  Tasks are organized into waves for parallel execution.
  
  REMEMBER:
  - Each task should be completable in 15-30 minutes
  - Mark parallel-safe tasks with [P]
  - Include exact file paths
  - Include verification commands
  - Test-first: Write tests before implementation
-->

**Feature Number**: NNN
**Feature Branch**: `NNN-[short-name]`
**Spec Reference**: `specs/NNN-[feature]/spec.md`
**Plan Reference**: `specs/NNN-[feature]/plan.md`
**Beastmode Tasks**: `.beastmode/state/plan/NNN-[feature].tasks.json`
**Status**: Generated → In Progress → Complete
**Created**: [DATE]

---

## Beastmode Metadata

### Execution Info
- **Current Wave**: [N]
- **Completed Tasks**: [N]/[Total]
- **Parallel Mode**: [Enabled/Disabled]
- **Worktree**: `.beastmode/worktrees/NNN-[feature]/`

### Wave Status
```
Wave 1: [████████░░] 80% (4/5 tasks)
Wave 2: [░░░░░░░░░░] 0% (0/3 tasks)
Wave 3: [░░░░░░░░░░] 0% (0/2 tasks)
```

---

## Wave 1: [Name] - Foundation

**Objective**: [What this wave achieves]
**Parallel-Safe**: [Yes/No]
**Status**: [Not Started/In Progress/Complete]

<!--
  If Parallel-Safe = Yes, these tasks can run simultaneously.
  Beastmode will verify no file overlap before spawning parallel agents.
-->

### Task 1.1: [Task Name]

**Priority**: P1
**Parallel**: [P] (if parallel-safe)
**Depends on**: [None or task IDs]
**Estimated Time**: [15-30 min]

**Spec Reference**: [FR-001, US-1]
**Plan Reference**: [Task 1.1 in plan.md]

**Files**:
- Create: `[exact/path/to/file]`
- Create: `[exact/path/to/test/file]` (Test-First!)
- Modify: `[exact/path/to/file]` (if any)

**Test-First Steps**:
1. **Write failing test**
   ```python
   # [Complete test code]
   def test_feature():
       result = function(input)
       assert result == expected
   ```

2. **Verify test fails**
   - Run: `[test command]`
   - Expected: FAIL with "[expected error]"

**Implementation Steps**:
3. **Implement feature**
   ```python
   # [Complete implementation code]
   def function(input):
       return result
   ```

4. **Verify test passes**
   - Run: `[test command]`
   - Expected: PASS

**Verification**:
```bash
# Run specific test
[command]

# Expected output:
# [expected output]
```

**Definition of Done**:
- [ ] Test written first
- [ ] Test fails initially (Red)
- [ ] Implementation complete
- [ ] Test passes (Green)
- [ ] Code follows project style
- [ ] No lint errors

---

### Task 1.2: [Task Name]

**Priority**: P1
**Parallel**: [P]
**Depends on**: [1.1 or None]
**Estimated Time**: [15-30 min]

**Files**:
- Create: `[path]`
- Create: `[test path]`

**Steps**:
1. [Step 1]
2. [Step 2]

**Verification**:
```bash
[command]
```

---

## Wave 2: [Name] - Core Functionality

**Objective**: [What this wave achieves]
**Parallel-Safe**: [Yes/No]
**Prerequisites**: [Wave 1 complete]
**Status**: [Not Started/In Progress/Complete]

### Task 2.1: [Task Name]

**Priority**: P1
**Parallel**: [P or -]
**Depends on**: [Wave 1]

**Files**:
- Create: `[path]`

**Steps**:
1. [Step 1]
2. [Step 2]

**Verification**:
```bash
[command]
```

---

### Task 2.2: [Task Name]

**Priority**: P2
**Parallel**: [P or -]
**Depends on**: [2.1]

**Files**:
- Modify: `[path]`

**Steps**:
1. [Step 1]

**Verification**:
```bash
[command]
```

---

## Wave 3: [Name] - Integration & Polish

**Objective**: [What this wave achieves]
**Parallel-Safe**: [Yes/No]
**Prerequisites**: [Wave 2 complete]
**Status**: [Not Started/In Progress/Complete]

### Task 3.1: [Task Name]

**Priority**: P2
**Parallel**: [P or -]
**Depends on**: [Wave 2]

**Files**:
- Create: `[path]`

**Steps**:
1. [Step 1]

**Verification**:
```bash
[command]
```

---

## Task Summary

### By Priority
| Priority | Count | Status |
|----------|-------|--------|
| P1 (Critical) | [N] | [N] complete |
| P2 (Important) | [N] | [N] complete |
| P3 (Nice) | [N] | [N] complete |

### By Wave
| Wave | Tasks | Parallel-Safe | Status |
|------|-------|---------------|--------|
| 1 | [N] | [Yes/No] | [% complete] |
| 2 | [N] | [Yes/No] | [% complete] |
| 3 | [N] | [Yes/No] | [% complete] |

### Parallel-Safe Tasks
<!-- List all [P] tasks for Beastmode swarm mode -->
- Wave 1: [1.1, 1.3] (if applicable)
- Wave 2: [2.1, 2.2, 2.3]
- Wave 3: [3.1]

**Total Parallel Groups**: [N]

---

## Beastmode Execution Format

### JSON Export

This file is automatically converted to `.beastmode/state/plan/NNN-[feature].tasks.json`:

```json
{
  "feature": "NNN-feature-name",
  "spec_file": "specs/NNN-feature/spec.md",
  "plan_file": "specs/NNN-feature/plan.md",
  "tasks_file": "specs/NNN-feature/tasks.md",
  "tasks": [
    {
      "id": "1.1",
      "name": "Task Name",
      "wave": 1,
      "priority": "P1",
      "parallel_safe": true,
      "depends_on": [],
      "files": ["path/to/file"],
      "test_files": ["path/to/test"],
      "status": "pending",
      "estimated_minutes": 20
    }
  ],
  "waves": {
    "1": {
      "name": "Foundation",
      "tasks": ["1.1", "1.2"],
      "parallel_safe": false,
      "prerequisites": []
    }
  },
  "stats": {
    "total_tasks": 10,
    "completed": 0,
    "in_progress": 0,
    "pending": 10
  }
}
```

---

## Execution Commands

### Sequential Mode
```
/speckit.implement NNN-feature-name
```

### Parallel Mode (Swarm)
```
/speckit.implement NNN-feature-name --parallel
```

### Resume Implementation
```
/speckit.implement NNN-feature-name --continue
```

---

## Verification Checklist

Before marking implementation complete:

- [ ] All P1 tasks complete
- [ ] All tests pass
- [ ] Contract tests pass
- [ ] Integration tests pass
- [ ] Quickstart validation passes
- [ ] Constitution gates passed
- [ ] No lint errors
- [ ] Documentation updated

---

## Handoff to Validation

When all tasks complete:

```
/speckit.validate NNN-feature-name
```

---

**Tasks Version**: 1.0  
**Last Updated**: [DATE]  
**Generated By**: /speckit.tasks
