# Numbers, units, and dates

Coder documentation uses digits for numbers 6 and higher in prose, allows either digits or words for 0 through 5, a non-breaking space between a number and its unit, and the `Month Day, Year` date format.
The rules on this page set those defaults.

## Digits for 6 and higher; 0 through 5 either way

Use digits for numbers 6 and higher in prose.
The traditional Chicago-style rule of "spell out one through nine" optimizes for print journalism.
Digits are more accessible for the international and non-native-English audience that reads Coder docs, scan faster in technical prose, and stay legible through machine translation.

Numbers 0 through 5 can go either way: write the digit or the word, whichever reads better in the sentence.
"6 months" and "90 days" read better as digits; "3 parameters" and "three parameters" both read fine, so this page doesn't force a choice below 6.

Two cases override that flexibility:

- **"More than X" and "X or more," for X 0 through 5**: always spell out the word ("more than one," "five or more"), never the digit ("more than 1," "5 or more"). Numbers 6 and higher keep the general digit rule in this construction too ("more than 6," "12 or more").
- **Literal technical values**, such as an exit code or a status code the reader matches or types verbatim: always use the digit, regardless of range, because the digit is the actual value ("exit code of 0," not "exit code of zero").

If a sentence would start with a digit 6 or higher, restructure the sentence so a word comes first.
Do not spell out the number to avoid the leading digit.
Spelling out a number 6 or higher reintroduces the rule the digits policy is meant to remove.

The rule covers counts, quantities, measurements, and values: the numbers a reader scans for.
Numbers that describe language itself stay spelled out regardless of value, like word counts in a grammar discussion ("a contraction joins exactly two words") and numbers mentioned as words ("spell out first through ninth").
Conceptual numbers (such as hundreds or thousands) or the specific decimal place of a rational number (such as tenths or hundredths) should always be spelled out.
Rewriting those as digits changes the register of the sentence without making it easier to scan.

**Do**:

> The agent retries 6 times before giving up.
>
> Workspaces auto-stop after 8 hours of inactivity.
>
> The template has 3 parameters.
>
> The template has three parameters.
>
> Configure more than one template for the group.
>
> Add five or more parameters before continuing.
>
> The CLI exits with an exit code of 0 on success and 1 on failure.
>
> Billions of users love Coder.

**Don't**:

> The agent retries six times before giving up.
>
> Workspaces auto-stop after eight hours of inactivity.
>
> 90 users connected to the workspace.
>
> Configure more than 1 template for the group.
>
> Add 5 or more parameters before continuing.
>
> The CLI exits with an exit code of zero on success and one on failure.
>
> 1,000,000,000s of users love Coder.

The first and second **Don't** examples spell out numbers 6 and higher.
The third example starts a sentence with a digit; restructure to put a word first ("The workspace has 90 connected users."), not spell out `90` to dodge the restructure.
The fourth and fifth use the digit inside a "more than X" / "X or more" construction, where 0 through 5 always spells out the word.
The sixth spells out `zero` and `one`, but both are literal exit-code values the reader matches against directly; use the digit there instead.

*Enforced by `Coder.DigitsSixPlus` (planned, ships at `warning` severity because the rule is preference, not hard policy).*

## Non-breaking space between number and unit

Insert a non-breaking space between a number and its unit so the pair never breaks across a line.
The Markdown source uses `&nbsp;` (HTML entity) or the Unicode character `U+00A0` (the literal non-breaking space).
The visible result is the same as a regular space, but the line breaker treats the number and unit as a single token.

**Do**:

In the Markdown source (what you type):

```md
The default timeout is 30&nbsp;seconds.
Connection latency under 150&nbsp;ms shows green.
```

In the rendered output (what the reader reads):

> The default timeout is 30&nbsp;seconds.
> Connection latency under 150&nbsp;ms shows green.

The rendered output looks identical to text written with a regular space.
The difference shows up only at the end of a line: the browser never splits `30` and `seconds` across 2 lines.
To check the behavior, shrink the browser window until the sentence wraps.
The number and the unit move to the next line together rather than separating.

**Don't**:

In the Markdown source:

```md
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
The format is unambiguous across locales, which the all-numeric forms (`07/31/2026` versus `31/07/2026`) aren't.

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
Use digits with a suffix for `10th` and higher.
Ordinals are the one place the digits policy makes an exception for numbers 6 through 9, spelling out the word instead of the digit form the cardinal-number rule would otherwise require, because spelled-out ordinals read more naturally in prose at low counts.

**Do**:

> The first time you run `coder login`, the CLI prompts you for an access URL.
>
> The 10th workspace in the list is the oldest.

**Don't**:

> The 1st time you run `coder login`, the CLI prompts you for an access URL.
>
> The tenth workspace in the list is the oldest.

*Enforced by a planned scoped fork of `Google.Ordinal`.
The stock rule spells out all ordinals, so it cannot enforce this policy as written.*

## Learn more

- [Style guide landing page](./README.md)
- [Capitalization and punctuation](./capitalization-and-punctuation.md)
- [Formatting](./formatting.md)
