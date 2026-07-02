# Numbers, units, and dates

Coder documentation uses digits for all numbers in prose, a non-breaking space between a number and its unit, and the `Month Day, Year` date format.
The rules on this page set those defaults.

## Digits everywhere

Use digits for all numbers in prose, including small whole numbers.
The traditional Chicago-style rule of "spell out one through nine" optimizes for print journalism.
Digits are more accessible for the international and non-native-English audience that reads Coder docs, scan faster in technical prose, and stay legible through machine translation.

If a sentence would start with a digit, restructure the sentence so a word comes first.
Do not spell out the number to avoid the leading digit.
That reintroduces the rule the digits-everywhere policy is meant to remove.

**Do**:

> The agent retries 3 times before giving up.
>
> Workspaces auto-stop after 8 hours of inactivity.
>
> The workspace has 5 connected users.

**Don't**:

> The agent retries three times before giving up.
>
> Workspaces auto-stop after eight hours of inactivity.
>
> 5 users connected to the workspace.

The first and second **Don't** examples spell out small numbers.
The third example starts a sentence with a digit.
Restructure to put a word first ("The workspace has 5 connected users.").

*Enforced by `Coder.DigitsEverywhere` (planned, ships at `warning` severity because the rule is preference, not hard policy).*

## Non-breaking space between number and unit

Insert a non-breaking space between a number and its unit so the pair never breaks across a line.
The Markdown source uses `&nbsp;` (HTML entity) or the Unicode character `U+00A0` (the literal non-breaking space).
The visible result is the same as a regular space, but the line breaker treats the number and unit as one token.

**Do**:

In the Markdown source (what you type):

```markdown
The default timeout is 30&nbsp;seconds.
Connection latency under 150&nbsp;ms shows green.
```

In the rendered output (what the reader reads):

> The default timeout is 30&nbsp;seconds.
> Connection latency under 150&nbsp;ms shows green.

The rendered output looks identical to text written with a regular space.
The difference shows up only at the end of a line: the browser will never split `30` and `seconds` across two lines.
To see the rule in action, shrink the browser window until the sentence wraps.
The number and the unit move to the next line together rather than separating.

**Don't**:

In the Markdown source:

```markdown
The default timeout is 30 seconds.
Connection latency under 150ms shows green.
```

The first line allows the browser to split `30` from `seconds`.
The second line omits the space entirely, which also reads worse.

In code blocks, configuration values, and CLI output, the original format is preserved (`30s`, `150ms`).
The non-breaking-space rule applies to prose only.

*Enforced by `Google.Units` (planned).*

## Date format

Write dates as `Month Day, Year` with a full month name and a comma between day and year.
The format is unambiguous across locales, which the all-numeric forms (`07/31/2026` versus `31/07/2026`) are not.

**Do**:

> Coder released version 2.20 on July 31, 2026.

**Don't**:

> Coder released version 2.20 on 07/31/2026.
>
> Coder released version 2.20 on 31 July 2026.
>
> Coder released version 2.20 on 2026-07-31.

In code blocks, configuration values, log lines, and API responses, keep whatever format the source uses.
ISO 8601 (`2026-07-31`) is correct in those contexts.

*Enforced by `Google.DateFormat` (planned).*

## Time format

Write times in 12-hour format with a space and uppercase AM or PM.

**Do**:

> The maintenance window starts at 9 AM and ends at 5 PM.

**Don't**:

> The maintenance window starts at 9am and ends at 5pm.
>
> The maintenance window starts at 09:00 and ends at 17:00.

In code blocks and timestamps from logs or APIs, keep the source format.
The 12-hour rule is for prose only.

*Enforced by `Google.AMPM` (planned).*

## Ordinals

Spell out ordinals `first` through `ninth`.
Use digits with a suffix for `10th` and up. This is the one place the digits-everywhere rule yields, because ordinals spelled out read more naturally in prose at low counts.

**Do**:

> The first time you run `coder login`, the CLI prompts you for an access URL.
>
> The 10th workspace in the list is the oldest.

**Don't**:

> The 1st time you run `coder login`, the CLI prompts you for an access URL.
>
> The tenth workspace in the list is the oldest.

*Enforced by `Google.Ordinal` (planned).*

## Related

- [Style guide landing page](./README.md)
- [Capitalization and punctuation](./capitalization-and-punctuation.md)
- [Formatting](./formatting.md)
