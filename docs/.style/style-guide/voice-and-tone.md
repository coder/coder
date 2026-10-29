---
review-personas: [language]
---

# Voice and tone

Coder documentation addresses the reader directly, uses active voice, and describes the product in the present tense.
The rules on this page set those defaults.

For pronoun conventions that center inclusive language, refer to [Accessibility and inclusion](./accessibility-and-inclusion.md).

## Address the reader directly

Use the second person ("you") in prose that gives the reader an instruction or describes what the reader sees, types, or gets back.
Second person is direct, scales across audiences, and avoids the ambiguity of "the user" (which user?) or generic constructions.

**Do**:

> You can connect to a workspace over SSH after you've installed the Coder CLI.

**Don't**:

> Users can connect to workspaces over SSH after the user has installed the Coder CLI.

*Documentation-only.
No Vale rule.*

## Avoid first-person singular

First-person singular pronouns (`I`, `my`, `me`, `mine`, `I'm`, `I've`) imply a single author speaking to a single reader, which is the wrong register for product documentation.
Rewrite in the second person or in a neutral voice.

**Do**:

> You can configure the workspace timeout in the template settings.

**Don't**:

> I usually set the workspace timeout in the template settings.

*Enforced by `Coder.FirstPersonSingular` (planned).*

## Reserve first-person plural for Coder Technologies

`We`, `us`, and `our` refer to **Coder Technologies, Inc.**, the company that makes the Coder platform.
For formal references to the company, use the full name ("Coder Technologies, Inc.").
For informal references in body prose, use `we`, `us`, or `our`.

Do not use first-person plural for:

- The product itself.
  The product is `Coder`, not "we".
  Rewrite to put the product, a release, or a feature as the subject.
- A combined "you and the docs" or "you and the author".
  That construction obscures who is taking the action.

**Do**:

> For more information about Premium licensing, [contact us](https://coder.com/contact/sales).
>
> Coder Technologies, Inc. publishes a new agent binary in each release.
>
> Each release includes a new agent binary.
> You can install the agent with the workspace template.

**Don't**:

> We ship a new agent binary in each release.
>
> Coder ships new agent binaries: we release them on the first of each month.
>
> We can install the agent by running this command on our workspace.

The first **Don't** uses "we" to mean the product rather than the company.
Rewrite with the product, release, or feature as the subject ("Each release includes ...").
The second **Don't** uses "we" to refer to the product's release behavior.
The third **Don't** uses "we" to mean "the docs and the reader together," which obscures who runs the command.

*Enforced by `Coder.FirstPersonPlural` (planned).*

## Active voice by default

Active voice puts the actor first and reads faster.
Passive voice is acceptable when the actor is genuinely unknown or irrelevant (`The token is rotated every 24 hours.`), but the default is active.

**Do**:

> Coder rotates the agent token every 24 hours.

**Don't**:

> The agent token is rotated every 24 hours by Coder.

*Documentation-only.
No Vale rule.
Imprecise rules like `Google.Passive` and `write-good.Passive` fire on every passive construction including the legitimate ones, so they stay out of the package per the rule-authoring doctrine.*

## Present tense by default

Describe how the product works in the present tense.
Future tense ("will") implies an event that hasn't happened yet at read time.
Reserve future tense for:

- Genuine future events, like scheduled rollouts or deprecations with a known date.
- Conditional or predictive statements where "will" carries the meaning "is guaranteed to".
  "If you stop the workspace, the agent will disconnect within 30 seconds" describes a guaranteed consequence and reads more naturally with `will` than with the present tense.

**Do**:

> The provisioner reads the template files and creates the workspace.
>
> If you delete the template, Coder will refuse to create new workspaces from it.

**Don't**:

> The provisioner will read the template files and will create the workspace.
>
> When you run the install script, it will download the latest release.

The second **Don't** uses future tense to describe normal behavior of the install script.
Use plain present tense for behavior the product already exhibits.

*Documentation-only.
No Vale rule.*

## Describe conditions, not moments in time

Words that place the reader in time, such as `now`, `new`, `currently`, `recently`, `soon`, and `no longer`, go stale when the next release ships.
They also give the reader nothing to act on.
State the condition that makes the behavior true instead: a version, a setting, a role, or a step the reader completes first.

**Do**:

> After you turn on the experiment, you can turn off automatic use for a skill.
>
> Starting in Coder v2.24, the agent reconnects automatically.

**Don't**:

> You can now turn off automatic use for a skill.
>
> The agent currently reconnects automatically.

In a sequenced tutorial, `now` can mark a point in the reader's own progress ("Now that you added a language, install your tools"), because it refers to the steps, not to a release.
For version history, refer to [Describe the current version, for the end user](../content-guidelines.md#describe-the-current-version-for-the-end-user).

*Documentation-only.
No Vale rule.*

## Keep sentences and paragraphs short

Aim for 25 words or fewer per sentence in body prose.
Steps in a procedure have a firm 20-word limit: refer to [Keep steps short](./procedural-writing.md#keep-steps-short).

Give each paragraph one topic and open with the sentence that states it.
Split a paragraph when it grows past roughly 6 sentences.

The body-prose budgets are targets, not ceilings.
A sentence that needs 30 words to be precise beats two vague short ones.

**Do**:

> The provisioner reads the template files and validates them against the schema.
> It then creates the workspace and starts the agent.

**Don't**:

> The provisioner, which reads the template files that the administrator pushed and validates them against the schema before it creates any resources, then creates the workspace and starts the agent.

*Adapted from ASD-STE100 Issue 9, rules 6.1 to 6.6.
Documentation-only.
No Vale rule.*

### How to count words

The sentence budget on this page and the step limit in [Keep steps short](./procedural-writing.md#keep-steps-short) count words the same way:

- An inline code span counts as one word, whatever it contains.
  Code spans hold commands, identifiers, and text the reader copies or types, and the reader handles each span as one unit.
- A bold UI label counts as one word, for the same reason: `**Create workspace**` is one word.
- A number and its unit count as one word: `30&nbsp;seconds`.
- A hyphenated compound counts as one word: `long-running`.
- Link text counts word by word, because the reader reads it as prose.

**Example**:

> Run `coder templates push --yes my-template` to publish the template.

The sentence has 6 words: `Run`, the code span, `to`, `publish`, `the`, and `template`.

*Not a rule.
This section defines how the word budgets in this guide count words.*

## Prefer verbs over noun forms

English can wrap an action in a noun ("perform the installation of") instead of stating the verb ("install").
The noun form adds words and hides the actor.
Use the verb.

**Do**:

> Before you remove the template, export its insights.
>
> Coder validates the token on each request.

**Don't**:

> Before the removal of the template, perform an export of its insights.
>
> Coder performs validation of the token on each request.

*Adapted from ASD-STE100 Issue 9, rule 3.7.
Documentation-only.
No Vale rule.*

## Give each pronoun one clear referent

A pronoun (`it`, `this`, `they`, `that`) must point to exactly one thing.
If the pronoun can point to two nouns, repeat the noun instead.
The repetition reads slightly worse to the writer and much clearer to the reader.

The same rule covers a bare `this` that summarizes a whole clause.
Name the thing that `this` refers to.

**Do**:

> Restart the server and the agent.
> The agent reconnects automatically.
>
> The build can fail when the quota is exhausted.
> This failure appears in the audit log.

**Don't**:

> Restart the server and the agent.
> It reconnects automatically.
>
> The build can fail when the quota is exhausted.
> This appears in the audit log.

*Adapted from ASD-STE100 Issue 9, general recommendations GR-3 and GR-4.
Documentation-only.
No Vale rule.*

## Contractions are the default

Contractions match how a reader's internal voice sounds.
Prefer them in body prose for the same reason the docs use second person and present tense.
Expand a contraction only when one of the following exceptions applies, or when the sentence is a negative instruction (refer to [Spell out negative instructions](#spell-out-negative-instructions)).

**Do**:

> Coder doesn't restart workspaces on minor template updates.
>
> If you're using PostgreSQL, set the connection string before you start `coder server`.

**Don't**:

> Coder does not restart workspaces on minor template updates.
>
> If you are using PostgreSQL, set the connection string before you start `coder server`.

**Auxiliary contractions need a complement.**
`you'd`, `there's`, `it's`, `we'd`, and `they're` carry an unspoken verb form, participle, or adjective.
They can't end a sentence because the elided word goes missing with them.
Negation contractions like `don't`, `won't`, and `can't` end sentences fine because the elided `not` is itself the complement.

**Do**:

> Restart the workspace if you want.
>
> The agent reattaches whenever a workspace exists.

**Don't**:

> Restart the workspace if you'd.
>
> The agent reattaches whenever there's.

**One contraction joins exactly two words.**
Triple-word contractions like `you'd've`, `wouldn't've`, and `shouldn't've` compress three words into one apostrophe-laden form.
Expand one of the two contractions, keeping whichever reads more naturally in context.

**Do**:

> If you would've finished the upgrade earlier, the migration wouldn't have failed.
>
> If you'd have finished the upgrade earlier, the migration wouldn't have failed.

**Don't**:

> If you'd've finished the upgrade earlier, the migration wouldn't've failed.

**Spell out for emphasis and high-stakes operations.**
Data loss, security warnings, and irreversible operations like deletions deserve the full visual weight of `do not`, `cannot`, and `will not`.
The contracted forms read fast and let a busy reader skip past the warning.

**Do**:

> Do not delete the workspace before backing up its state.
> Coder cannot restore a deleted workspace.
>
> You cannot undo `coder delete`.

**Don't**:

> Don't delete the workspace before backing up its state.
> Coder can't restore a deleted workspace.
>
> You can't undo `coder delete`.

**The default trades against the international audience.**
Plain-language guidance for non-native readers, including [ASD-STE100](https://www.asd-ste100.org/), bans contractions because the expanded forms are simpler to parse and to machine-translate.
The Coder docs keep contractions because the genre reads conversationally and the [reading-level target](./accessibility-and-inclusion.md#reading-level) already bounds sentence complexity.
The high-stakes exception earlier in this section applies the STE logic exactly where a misreading costs the most.

*Documentation-only.
No Vale rule.*

## Spell out negative instructions

A contraction is fine in a statement, but not in an instruction.
When the sentence tells the reader not to do something, write `do not`.
A reader who skims past the `n't` in `don't` reads the opposite instruction.

The rule follows the mood of the sentence, not the topic.
A statement about what the product does or doesn't do keeps the contraction default from [Contractions are the default](#contractions-are-the-default).

**Do**:

> Do not delete the production database.
>
> If the Terraform files have a syntax error, Coder won't push the new version of the template.
>
> You don't need a license to start a workspace.

**Don't**:

> Don't delete the production database.
>
> Run `make gen`, and don't edit the generated file.

*Enforced by `Coder.ImperativeNegation` (`warning`).
The rule flags `don't` only where it opens an imperative, so declaratives such as "You don't need" stay clean.*

## Leave out "please"

Documentation gives instructions, not requests.
"Please" adds a word to every instruction and makes a required step read as optional.
Write the instruction directly.

When you quote product text that contains "please", such as an error message, put the quoted text in a code span, as [Code font](./formatting.md#code-font) describes for literal product text.

**Do**:

> Restart the server after you change the configuration.
>
> If the build fails with `Please try again later`, wait 5&nbsp;minutes and run the build again.

**Don't**:

> Please restart the server after you change the configuration.
>
> For more information, please contact your administrator.

*Enforced by `Coder.Please` (`warning`).
The rule skips code spans and fenced code blocks, so quoted product text in code font isn't flagged.*

## Trailing prepositions are a judgment call

A sentence that ends with a preposition (`with`, `to`, `from`, `for`, `on`, `of`, `at`, `by`, `into`, `over`, `under`, `about`) can leave its object implicit, which adds a small comprehension cost.
Avoiding the trailing preposition, though, can produce a more awkward sentence.
No single rule covers every case.
Read both versions and keep the one that reads more naturally.

Lean toward rewriting when the trailing preposition is redundant, or when the reordered version still reads naturally:

**Do**:

> The CLI prompts you for the directory in which to store the template.
>
> Where is the config file?

**Don't**:

> The CLI prompts you for the directory you want to store the template in.
>
> Where is the config file at?

The second **Don't** keeps a redundant `at`.
"Where is the config file?" says the same thing.

Keep the trailing preposition when avoiding it contorts the sentence:

**Do**:

> That's an error you can't recover from.
>
> Open the repository you want to clone from.

**Don't**:

> That's an error from which you can't recover.
>
> Open the repository from which you want to clone.

The first **Don't** is the classic over-correction: the rewrite is harder to read than the preposition it avoids.

When both versions read equally well, the writer chooses.
Treat avoiding a trailing preposition as a default to prefer, not a rule to enforce.

*Documentation-only.
No Vale rule.*

## Learn more

- [Style guide landing page](./README.md)
- [Word choice](./word-choice.md)
- [Accessibility and inclusion](./accessibility-and-inclusion.md)
