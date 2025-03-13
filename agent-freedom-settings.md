# Agent Freedom Settings

## Overview

Agent Freedom Settings control the level of freedom (autonomy) given to AI assistants when interacting with coder repositories. This document explains the various settings and how to configure them.

## Freedom Levels

Freedom levels are expressed on a scale of 0-100, with higher numbers indicating more autonomy:

| Level | Description | Best for |
|-------|-------------|----------|
| 0-20  | Restrictive | Security-critical code, infrastructure-as-code |
| 21-40 | Limited | Core business logic, authentication, payments |
| 41-60 | Balanced | Standard feature development, bug fixes |
| 61-80 | Enhanced | UI components, utility functions, tests |
| 81-100 | Proactive | Documentation, comments, non-critical code |

## Preset Values

We offer preset values to make it easier to select an appropriate freedom level:

| Preset | Value | Description |
|--------|-------|-------------|
| Restrictive | 20/100 | Minimal autonomy, requires explicit approval for nearly all actions |
| Limited | 40/100 | Limited autonomy for simple tasks with careful oversight |
| Balanced | 60/100 | Balanced approach suitable for most development tasks |
| Enhanced | 80/100 | Increased autonomy for faster development with less oversight |
| Proactive | 88/100 | High autonomy for quick iterations while maintaining some guardrails |
| Unrestricted | 100/100 | Maximum autonomy, minimal oversight |

## Setting Agent Freedom

You can set the agent freedom level in your PR or issue template by adding a tag in this format:

```
[Agent Freedom Setting: Proactive (88/100)]
```

## Common Mismatches

A "mismatch" occurs when the selected preset doesn't match the numeric value provided:

| Incorrect | Correct |
|-----------|---------|
| [Agent Freedom Setting: Proactive (80/100)] | [Agent Freedom Setting: Enhanced (80/100)] |
| [Agent Freedom Setting: Proactive (88/100)] | [Agent Freedom Setting: Proactive (88/100)] ✓ |
| [Agent Freedom Setting: Proactive (100/100)] | [Agent Freedom Setting: Unrestricted (100/100)] |

## Recommendations

- For general development work, use **Balanced (60/100)** or **Enhanced (80/100)**
- For documentation and non-critical code, use **Proactive (88/100)**
- For security-sensitive code, use **Restrictive (20/100)** or **Limited (40/100)**

## Examples

### Pull Request Template Example

```markdown
## Description
[Describe your changes here]

## Related Issues
[Link to any related issues]

## Agent Freedom Setting
[Agent Freedom Setting: Proactive (88/100)]

## Testing
[Describe how you tested your changes]
```

### Issue Template Example

```markdown
## Description
[Describe the issue here]

## Expected Behavior
[What should happen]

## Current Behavior
[What is happening now]

## Agent Freedom Setting
[Agent Freedom Setting: Enhanced (80/100)]

## Additional Information
[Any other relevant information]
```