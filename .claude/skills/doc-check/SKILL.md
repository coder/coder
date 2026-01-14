---
name: doc-check
description: Checks if code changes require documentation updates
---

# Documentation Check Skill

Review code changes and determine if documentation updates or new documentation
is needed.

## Setup for PR Review

When given a PR number, get the PR info and changes:

```bash
cd ~/coder

# Get PR details (title, description, author)
gh pr view <PR_NUMBER> --repo coder/coder

# View changed files
gh pr diff <PR_NUMBER> --repo coder/coder --name-only

# View the full diff
gh pr diff <PR_NUMBER> --repo coder/coder
```

## Workflow

1. **Check the PR title first** - If it's clearly not user-facing (refactor, test,
   chore, internal, typo fix), skip the full review and post a quick comment
   that no documentation updates are needed
2. **Read the code changes** to understand what's new or modified
3. **Search the docs** for related content using grep, find, or by reading files
4. **Decide what's needed**:
   - Do existing docs need updates to match the code?
   - Is new documentation needed for undocumented features?
   - Or is everything already covered?
5. **Post a comment** on the PR with your findings

## Trigger Context

The task prompt will include context about why the review was triggered:

- **NEW PR**: First review of this PR. Do a full initial review.
- **PR UPDATED**: New commits were pushed. Focus on what changed since last
  review - check if previous recommendations were addressed or if new changes
  need documentation.
- **REQUESTED via label**: Someone manually requested a review. Do a thorough
  review.
- **MANUAL**: Workflow was manually triggered. Do a thorough review.

Adjust your comment accordingly - for PR updates, acknowledge if previous
feedback was addressed.

### Skip doc-check for

- Refactors with no behavior change
- Test-only changes
- Internal/chore changes
- Typo fixes in code
- Dependency updates

## What to Check

- **Accuracy**: Does documentation match current code behavior?
- **Completeness**: Are new features/options documented?
- **Examples**: Do code examples still work?
- **CLI/API changes**: Are new flags, endpoints, or options documented?

## Output Format

```markdown
## Documentation Check

Reviewed docs related to [brief description of code changes].

**Updates needed:**
- [ ] docs/admin/users.md:L42 - Parameter name changed from X to Y
- [ ] docs/cli/server.md:L15 - Missing new --timeout flag

**New documentation needed:**
- [ ] docs/admin/notifications.md - New notification system is undocumented

**Suggested fixes:**
[Include specific suggestions using GitHub suggestion format]
```

If no changes needed:

```markdown
## Documentation Check

Reviewed docs related to [brief description].

**No documentation updates needed** - existing docs accurately reflect the
code changes.
```

If skipped based on PR title:

```markdown
## Documentation Check

**Skipped** - This PR appears to be [refactor/test/chore] based on the title
and is unlikely to need documentation updates.
```

For PR updates (re-review after new commits):

```markdown
## Documentation Check (Update)

Reviewed latest changes to PR.

**Previous feedback status:**
- ✅ docs/admin/users.md update - addressed in latest commit
- ⏳ docs/cli/server.md - still needs --timeout flag documented

**New changes:**
- No additional documentation updates needed from latest commits
```

## Common Issues

- Code examples that no longer work
- Missing documentation for new features
- New CLI flags/options not documented
- Changed default values not reflected
- Outdated screenshots or version numbers
