---
name: refine-plan
description: Iteratively refine development plans using TDD methodology. Ensures plans are clear, actionable, and include red-green-refactor cycles with proper test coverage.
---

# Refine Development Plan

## Overview

Good plans eliminate ambiguity through clear requirements, break work into clear phases, and always include refactoring to capture implementation insights.

## When to Use This Skill

| Symptom                     | Example                                |
|-----------------------------|----------------------------------------|
| Unclear acceptance criteria | No definition of "done"                |
| Vague implementation        | Missing concrete steps or file changes |
| Missing/undefined tests     | Tests mentioned only as afterthought   |
| Absent refactor phase       | No plan to improve code after it works |
| Ambiguous requirements      | Multiple interpretations possible      |
| Missing verification        | No way to confirm the change works     |

## Planning Principles

### 1. Plans Must Be Actionable and Unambiguous

Every step should be concrete enough that another agent could execute it without guessing.

- ❌ "Improve error handling" → ✓ "Add try-catch to API calls in user-service.ts, return 400 with error message"
- ❌ "Update tests" → ✓ "Add test case to auth.test.ts: 'should reject expired tokens with 401'"

NEVER include thinking output or other stream-of-consciousness prose mid-plan.

### 2. Push Back on Unclear Requirements

When requirements are ambiguous, ask questions before proceeding.

### 3. Tests Define Requirements

Writing test cases forces disambiguation. Use test definition as a requirements clarification tool.

### 4. TDD is Non-Negotiable

All plans follow: **Red → Green → Refactor**. The refactor phase is MANDATORY.

## The TDD Workflow

### Red Phase: Write Failing Tests First

**Purpose:** Define success criteria through concrete test cases.

**What to test:**

- Happy path (normal usage), edge cases (boundaries, empty/null), error conditions (invalid input, failures), integration points

**Test types:**

- Unit tests: Individual functions in isolation (most tests should be these - fast, focused)
- Integration tests: Component interactions (use for critical paths)
- E2E tests: Complete workflows (use sparingly)

**Write descriptive test cases:**

**If you can't write the test, you don't understand the requirement and MUST ask for clarification.**

### Green Phase: Make Tests Pass

**Purpose:** Implement minimal working solution.

Focus on correctness first. Hardcode if needed. Add just enough logic. Resist urge to "improve" code. Run tests frequently.

### Refactor Phase: Improve the Implementation

**Purpose:** Apply insights gained during implementation.

**This phase is MANDATORY.** During implementation you'll discover better structure, repeated patterns, and simplification opportunities.

**When to Extract vs Keep Duplication:**

This is highly subjective, so use the following rules of thumb combined with good judgement:

1) Follow the "rule of three": if the exact 10+ lines are repeated verbatim 3+ times, extract it.
2) The "wrong abstraction" is harder to fix than duplication.
3) If extraction would harm readability, prefer duplication.

**Common refactorings:**

- Rename for clarity
- Simplify complex conditionals
- Extract repeated code (if meets criteria above)
- Apply design patterns

**Constraints:**

- All tests must still pass after refactoring
- Don't add new features (that's a new Red phase)

## Plan Refinement Process

### Step 1: Review Current Plan for Completeness

- [ ] Clear context explaining why
- [ ] Specific, unambiguous requirements
- [ ] Test cases defined before implementation
- [ ] Step-by-step implementation approach
- [ ] Explicit refactor phase
- [ ] Verification steps

### Step 2: Identify Gaps

Look for missing tests, vague steps, no refactor phase, ambiguous requirements, missing verification.

### Step 3: Handle Unclear Requirements

If you can't write the plan without this information, ask the user. Otherwise, make reasonable assumptions and note them in the plan.

### Step 4: Define Test Cases

For each requirement, write concrete test cases. If you struggle to write test cases, you need more clarification.

### Step 5: Structure with Red-Green-Refactor

Organize the plan into three explicit phases.

### Step 6: Add Verification Steps

Specify how to confirm the change works (automated tests + manual checks).

## Tips for Success

1. **Start with tests:** If you can't write the test, you don't understand the requirement.
2. **Be specific:** "Update API" is not a step. "Add error handling to POST /users endpoint" is.
3. **Always refactor:** Even if code looks good, ask "How could this be clearer?"
4. **Question everything:** Ambiguity is the enemy.
5. **Think in phases:** Red → Green → Refactor.
6. **Keep plans manageable:** If plan exceeds ~10 files or >5 phases, consider splitting.

---

**Remember:** A good plan makes implementation straightforward. A vague plan leads to confusion, rework, and bugs.
