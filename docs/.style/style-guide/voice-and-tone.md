# Voice and tone

Coder documentation addresses the reader directly, uses active voice, and describes the product in the present tense.
The rules on this page set those defaults.

For pronoun conventions that center inclusive language, refer to [Accessibility and inclusion](./accessibility-and-inclusion.md).

## Address the reader directly

Use the second person ("you") in prose that gives the reader an instruction or describes what the reader sees, types, or gets back.
Second person is direct, scales across audiences, and avoids the ambiguity of "the user" (which user?) or generic constructions.

**Do**:

> You can connect to a workspace over SSH after you have installed the Coder CLI.

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

*Enforced by `Coder.FirstPersonSingular`.*

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

> For more information about enterprise licensing, [contact us](https://coder.com/contact/sales).
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
The third **Don't** uses "we" to mean "the docs and the reader together", which obscures who runs the command.

*Enforced by `Coder.FirstPersonPlural`.*

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
Future tense ("will") implies an event that has not happened yet at read time.
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

## Trailing prepositions are a judgment call

A sentence that ends with a preposition (`with`, `to`, `from`, `for`, `on`, `of`, `at`, `by`, `into`, `over`, `under`, `about`) can leave its object implicit, which adds a small comprehension cost.
Avoiding the trailing preposition, though, can produce a more awkward sentence.
There is no one-size-fits-all rule.
Read both versions and keep the one that reads more naturally.

Lean toward rewriting when the trailing preposition is redundant, or when the reordered version is still easy to read:

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

> This is some nonsense that I will not put up with.
>
> Open the repository you want to clone from.

**Don't**:

> This is some nonsense up with which I will not put.
>
> Open the repository from which you want to clone.

The first **Don't** is the classic over-correction: the rewrite is harder to read than the preposition it avoids.

When both versions read equally well, the writer chooses.
Treat avoiding a trailing preposition as a default to reach for, not a rule to enforce.

*Documentation-only.
No Vale rule.*

## Related

- [Style guide landing page](./README.md)
- [Word choice](./word-choice.md)
- [Accessibility and inclusion](./accessibility-and-inclusion.md)
