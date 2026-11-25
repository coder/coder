---
review-personas: [structure, language]
---

# Procedural writing

Procedural prose tells the reader what to do: the numbered steps of a how-to guide, a tutorial, or a Quickstart.
The rules on this page govern how instructions, conditions, and callouts behave inside a procedure.
For voice, tense, and sentence-level defaults that apply to all prose, refer to [Voice and tone](./voice-and-tone.md).
For the callout syntax and severity table, refer to [Formatting](./formatting.md#callouts).

Rules adapted from an external standard cite the source so an editor can consult the original rationale.
The source for most of this page is [ASD-STE100 Simplified Technical English](https://www.asd-ste100.org/) (Issue 9, 2025), the controlled-language standard for aerospace and defense maintenance documentation.
STE optimizes for readers who must never misread an instruction.
The Coder docs adopt its procedure-level discipline and a few of its word-level rules, such as [Choose words that have one meaning](./word-choice.md#choose-words-that-have-one-meaning).
The full controlled dictionary stays out, because it serves that stricter audience.

## One instruction per step

Each numbered step contains one action.
A reader executes a procedure one step at a time.
A step that bundles two actions invites the reader to complete the first and miss the second.

Two actions belong in one step only when they happen at the same time.

**Do**:

```md
1. Run `coder login`.
2. Paste the session token into the terminal prompt.
```

```md
1. Run `coder port-forward` and leave it running while you test the connection.
```

The second example stays a single step because its actions overlap in time.

**Don't**:

```md
1. Run `coder login` and paste the session token into the terminal prompt.
```

The actions happen in sequence, so they belong in separate steps.

*Adapted from ASD-STE100 Issue 9, rule 5.2.
Documentation-only.
No Vale rule.*

## Put the condition before the instruction

When a step applies only under a condition, state the condition first.
The reader acts as they read.
A condition placed after the command reaches the reader after they have started the action.

The same order applies to prerequisites: name the required state before the step that depends on it.

**Do**:

> If the workspace is running, stop it before you push the template.
>
> When the build completes, select **Open in VS Code**.

**Don't**:

> Stop the workspace before you push the template if it's running.
>
> Select **Open in VS Code** when the build completes.

*Adapted from ASD-STE100 Issue 9, rule 5.4.
Documentation-only.
No Vale rule.*

## Keep steps short

Write the instruction of each step in 20 words or fewer.
The instruction is the step's first paragraph: the action, and an expected result if the step states one.
Paragraphs below the instruction explain the step, and they follow the body prose target instead.
Code blocks, images, and tables inside a step don't count toward the limit.
Count words as [How to count words](./voice-and-tone.md#how-to-count-words) describes.
Body prose gets a flexible 25-word target (refer to [Keep sentences and paragraphs short](./voice-and-tone.md#keep-sentences-and-paragraphs-short)).
Instructions get a firm limit because the reader is mid-task and holds the instruction in working memory while they act on it.

When an instruction needs more words, shorten it, or move the explanation to a paragraph below the instruction.
When the step holds more than one action, split it into one step for each action.
Keep linked information together: do not break one action or one explanation across paragraphs only to meet the limit.
Do not cut words that carry meaning to meet the limit.

**Do**:

```md
1. Open **Templates** > **Settings** > **Schedule**.
2. Set the autostop timer to 8 hours.

   Workspaces built from the template then stop overnight, which saves infrastructure cost while nobody uses them.
```

**Don't**:

```md
1. Open the template settings page, find the **Schedule** section, and set the autostop timer to 8 hours so workspaces stop overnight.
```

*Adapted from ASD-STE100 Issue 9, rule 5.1.
Enforced by `Coder.StepLength` (`warning`).
The rule counts the words in the first paragraph of each ordered-list step and flags instructions over 20.
It approximates [How to count words](./voice-and-tone.md#how-to-count-words), and its header comment lists the approximations.*

## Use a full clause after before, after, when, and while

In a step, follow `before`, `after`, `when`, and `while` with a full clause that names who acts.
The `-ing` form hides the subject, and the reader must work out who does the action.

**Do**:

```md
1. Before you start the server, set `CODER_ACCESS_URL`.
```

**Don't**:

```md
1. Before starting the server, set `CODER_ACCESS_URL`.
```

The rule covers procedure steps.
In body prose, either form is fine.

*Adapted from ASD-STE100 Issue 9, rule 3.5.
Documentation-only.
No Vale rule.*

## Callouts inform, steps instruct

A `NOTE` or `TIP` callout carries supplementary information.
It must not carry a required action, a limit, or an acceptance criterion.
Readers who skim a procedure skip its callouts, so everything the procedure requires must live in a numbered step.

- A required action becomes a step.
- A limit or an expected result goes in the step it validates, directly after the action.
- Information that prevents data loss, downtime, or a security exposure becomes a `WARNING` or `CAUTION`.

To test a procedure, read it with every `NOTE` and `TIP` deleted.
If the reader can no longer complete the procedure correctly, a callout is carrying required content.
Move that content into a step and test again.

**Do**:

```md
1. Back up the database.
2. Run the migration.
```

**Don't**:

```md
1. Run the migration.

> [!NOTE]
> Back up the database before you run the migration.
```

*Adapted from ASD-STE100 Issue 9, rule 5.5.
Documentation-only.
No Vale rule: Vale can't tell whether a callout carries a required action, so reviewers check this rule.*

## State the consequence in warnings

A `WARNING` or `CAUTION` callout has two parts: the instruction or condition, then the consequence of ignoring it.
A warning that names no consequence reads as decoration, and the reader cannot weigh a risk the page doesn't name.

**Do**:

```md
> [!WARNING]
> Do not delete the workspace before you back up its state.
> Coder cannot restore a deleted workspace.
```

**Don't**:

```md
> [!WARNING]
> Be careful when you delete workspaces.
```

*Adapted from ASD-STE100 Issue 9, rules 7.2 and 7.3.
Documentation-only.
No Vale rule.*

## Learn more

- [Style guide landing page](./README.md)
- [Voice and tone](./voice-and-tone.md)
- [Formatting](./formatting.md)
- [Word choice](./word-choice.md)
