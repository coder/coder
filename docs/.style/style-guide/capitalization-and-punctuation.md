# Capitalization and punctuation

Coder documentation uses sentence-case headings, the Oxford comma, US-style quotation, and no em-dashes or en-dashes in prose.
The rules on this page set those defaults.

For heading structure (H1, H2, H3 placement and order), refer to [Accessibility and inclusion](./accessibility-and-inclusion.md#heading-structure-and-placement).

## Sentence-case headings

Capitalize the first word of a heading or page title, plus any proper nouns.
Everything else is lowercase.
This rule covers H1 through H6 and matches the way the heading reads aloud.

**Do**:

```md
# Configure your workspace
## Set up SSH access
### Connect through JetBrains Toolbox
```

**Don't**:

```md
# Configure Your Workspace
## Set Up SSH Access
### Connect Through JetBrains Toolbox
```

*Enforced by `Google.Headings` (scope adjusted to skip CLI flag fragments and acronyms).*

## No gerund-leading headings

Do not start a heading with a present participle or gerund (an `-ing` word acting as a verb form).
The imperative form reads better for task headings.
The noun form reads better for concept headings.
Reserve gerund-leading headings for the rare case where neither alternative reads cleanly.

**Do**:

```md
## Install Coder
## Installation
## Configure your workspace
## Configuration reference
```

**Don't**:

```md
## Installing Coder
## Configuring your workspace
```

### Exceptions

Not every `-ing` word is a gerund-leading violation.
The rule targets verb forms (`installing`, `configuring`, `deploying`), not the following:

- Nouns that happen to end in `-ing` and have no verb counterpart in the heading: `String formatting`, `Heading structure`.
- Compound nouns where the `-ing` word names a category or feature: `Pricing`, `Billing`, `Logging`, `Monitoring`, `Tracing`, `Networking`.
- Adjectives derived from verbs that modify the head noun: `Running workspaces`, `Pending invitations`.

When the `-ing` word is the actual subject the section describes (a feature, a noun, or an attribute), the heading is fine.
When the `-ing` word is the verb form of a task the section walks through, rewrite as an imperative or as the noun form.

*Enforced by `Coder.GerundHeading`, with the exceptions listed earlier scoped in the rule.*

## No trailing punctuation in headings

Headings are labels, not sentences.
Drop terminal periods and exclamation points.
Use trailing question marks sparingly, and only when the heading is an actual question that the section answers.

The rule has scoped exceptions:

- **Periods (`.`) and exclamation points (`!`) inside backticks** are allowed when the heading names a literal identifier that contains the character (a config file ending in `.yml`, a CLI flag like `--force!`, a programming macro like `panic!`).
  The backticks tell the reader the punctuation is part of the identifier, not a sentence ender.
- **Question marks (`?`) inside backticks** are also allowed for the same reason (a query operator, a regex modifier, a UI element literally named `?`).

**Do**:

```md
## What's a workspace
## Quick reference
## What does the `panic!` macro do?
## Configure the `.vale.ini` file
```

**Don't**:

```md
## What's a workspace?
## Quick reference!
## Workspaces are great!
## Configure your workspace.
```

The first **Don't** uses a trailing question mark for a label that isn't actually a question.
Reword as a noun phrase ("What a workspace is") or drop the question mark.
The second and third are decorative.
The fourth treats the heading as a sentence.

*Periods and exclamation points enforced by `Google.HeadingPunctuation` at `error` severity.
Question marks enforced by `Google.HeadingPunctuation` at `suggestion` severity.
Both ignore characters inside backticks.*

## No em-dashes or en-dashes

Em-dashes (&mdash;, U+2014), en-dashes (&ndash;, U+2013), and the ASCII `--` fallback are banned in prose.
Em-dashes typically set off a parenthetical aside or a break in thought.
Replace them with commas (for a tight aside), parentheses (for a clearly secondary aside), or a period and a new sentence (for a thought that stands on its own).

**Do**:

> The provisioner, which Coder builds on top of Terraform, creates the workspace.
>
> The provisioner (which Coder builds on top of Terraform) creates the workspace.
>
> The provisioner creates the workspace.
> Coder builds the provisioner on top of Terraform.

**Don't**:

> The provisioner&mdash;which Coder builds on top of Terraform&mdash;creates the workspace.
>
> The provisioner -- which Coder builds on top of Terraform -- creates the workspace.

*Enforced by `scripts/check_emdash.sh` (existing CI script) and `Coder.EmDash` (planned).*

## Commas

### Comma after an introductory element

Place a comma after an introductory word, phrase, or clause that comes before the main clause.
The comma marks where the introduction ends and the main clause begins.

**Do**:

> In this guide, you add Ruby as a parameter option.
>
> After you authorize Coder, the workspace starts.
>
> To pull the template, run `coder templates pull`.

**Don't**:

> In this guide you add Ruby as a parameter option.
>
> After you authorize Coder the workspace starts.

*Documentation-only.
No Vale rule.*

### No comma in a short compound predicate

When `and`, `or`, or `but` joins two verbs that share one subject, don't put a comma before the conjunction.
The comma belongs there only when the conjunction joins two independent clauses, each with its own subject.

**Do**:

> Log in to Coder and select **Templates**.
>
> The agent opens a tunnel and forwards traffic over it.

**Don't**:

> Log in to Coder, and select **Templates**.
>
> The agent opens a tunnel, and forwards traffic over it.

When each side of the conjunction is a full clause with its own subject, the comma returns:

> Log in to Coder, and the dashboard opens.

*Documentation-only.
No Vale rule.*

### Oxford comma

Use a comma before the conjunction in a list of three or more items.

**Do**:

> The provisioner builds, configures, and starts the workspace.

**Don't**:

> The provisioner builds, configures and starts the workspace.

*Enforced by `Google.OxfordComma`.*

## US-style quotation

Place commas and periods inside closing quotation marks.
Semicolons and colons stay outside.
This is the United States convention and matches the dominant style of the surrounding tech-docs ecosystem.

**Do**:

> The error message reads, "workspace not found."

**Don't**:

> The error message reads, "workspace not found".

*Enforced by `Google.Quotes`.*

## Semicolons sparingly

Prefer two sentences.
A semicolon joins two complete thoughts when they're tightly related and a period would lose the connection, but in technical prose two sentences almost always read more clearly.

**Do**:

> The provisioner uses Terraform.
> It reads the template files and creates the workspace.

**Don't**:

> The provisioner uses Terraform; it reads the template files and creates the workspace.

*Documentation-only.
No Vale rule.*

## Exclamation points rare in prose

Exclamation points in body prose read as marketing copy or shouted emphasis.
Reserve them for code blocks, direct quotes from error messages, and rare moments where genuine emphasis serves the reader.

**Do**:

> Coder is ready to use.

**Don't**:

> Coder is ready to use!

*Enforced by `Google.Exclamation`.*

## Numeric ranges

Spell out the joiner in prose.
Use `5 to 10` or `between 5 and 10`, not `5-10`.
In code blocks, terse reference material, and tables where space matters, the hyphenated form is acceptable.

**Do**:

> The agent retries 5 to 10 times before giving up.

**Don't**:

> The agent retries 5-10 times before giving up.

*Enforced by `Google.Ranges`.*

## Related

- [Style guide landing page](./README.md)
- [Accessibility and inclusion](./accessibility-and-inclusion.md)
- [Formatting](./formatting.md)
- [Numbers, units, and dates](./numbers-units-and-dates.md)
