# Word choice

Coder documentation uses canonical brand and product names, plain language for product actions, and "refer to" instead of "see" for navigational pointers.
The rules on this page set those defaults.

For inclusive-language substitutions like `allowlist` or `primary`, refer to [Accessibility and inclusion](./accessibility-and-inclusion.md).

## Coder product and feature names

`Coder`, the company and the product, is always capitalized.
Feature names are capitalized as proper nouns when the prose names the feature.
The underlying generic concept stays lowercase.

When the prose refers to the Coder command-line interface as a tool, wrap it in backticks: `coder`.
The bare lowercase `coder` (no backticks) is wrong.
It reads as a misspelling of the product name.

| Do                              | Don't                                          |
|---------------------------------|------------------------------------------------|
| Coder                           | coder (referring to the product, no backticks) |
| `coder` (the CLI, in backticks) | coder (the CLI, no backticks)                  |
| AI Gateway                      | AI gateway, AIGateway, AI Bridge               |
| Workspace Proxy                 | workspace proxy (referring to the feature)     |
| workspace                       | Workspace (referring to the generic concept)   |
| template                        | Template (referring to the generic concept)    |
| agent                           | Agent (referring to the generic concept)       |
| provisioner                     | Provisioner (referring to the generic concept) |

**Do**:

> Run `coder login` to authenticate against the Coder server.
>
> Open the AI Gateway integration page to configure model providers.

**Don't**:

> Run coder login to authenticate against the coder server.
>
> Open the ai gateway integration page to configure model providers.

*Enforced by `Coder.ProductTerms` (planned).*

The [glossary](../../reference/glossary.md) is the fuller registry of these names and disambiguates collisions like the several senses of "agent".
When you add, rename, or deprecate a product or feature name, update the glossary in the same change.
The planned `Coder.ProductTerms` rule and the glossary should draw on one shared term list.

## Brand names

Use the canonical casing for third-party brand and product names.
The Coder docs team keeps a substitution list.

When the prose refers to a third-party command-line tool, wrap the tool name in backticks the same way as for the Coder CLI.
The product name (`Terraform`) stays capitalized in prose.
The CLI tool (`terraform`) lives in backticks.

| Do                                  | Don't                                 |
|-------------------------------------|---------------------------------------|
| HashiCorp                           | Hashicorp, HASHICORP                  |
| GitHub                              | Github, GITHUB                        |
| OpenTofu                            | Opentofu, OpenTOFU                    |
| Kubernetes                          | kubernetes (in prose), K8s (in prose) |
| Terraform                           | terraform (in prose, no backticks)    |
| `terraform` (the CLI, in backticks) | terraform (the CLI, no backticks)     |
| JetBrains                           | Jetbrains, jetbrains                  |
| VS Code                             | VSCode, VSC, VS code                  |

Lowercase forms remain correct in code blocks, URLs, package names, and Terraform provider sources, where the canonical form is lowercase by convention.

*Enforced by `Coder.BrandNames`.*

## Dev container terminology

The open standard at [containers.dev](https://containers.dev/) uses two forms in its own documentation:

- **`Development Container Specification`** (or **`Dev Container Spec`** for short) when naming the open specification, the Features ecosystem, or the Templates ecosystem.
- **`dev container`** (lowercase, two words) for the category and for any instance.

The Coder docs follow the same conventions.

`envbuilder` is the implementation tool Coder uses to build dev containers.
It isn't itself the concept, so it stays in backticks as a tool name.

> [!NOTE]
> The Coder feature that integrates the open standard with Coder workspaces is named `Dev Containers` in product context.
> The capitalization there comes from the [Coder product and feature names](#coder-product-and-feature-names) rule for Coder features, not from the underlying concept.

| Do                                  | Don't                                                |
|-------------------------------------|------------------------------------------------------|
| Development Container Specification | DevContainer Specification                           |
| Dev Container Spec                  | dev container spec (as the proper-noun shorthand)    |
| dev container                       | Dev Container, DevContainer, devcontainer (in prose) |
| `devcontainer.json`                 | `dev-container.json`, `DevContainer.json`            |
| `envbuilder`                        | EnvBuilder, Envbuilder, env builder                  |

**Do**:

> Coder builds dev containers that conform to the Development Container Specification.
> The template defines the dev container in a `devcontainer.json` file.
> The provisioner builds the dev container with `envbuilder` and starts the agent inside it.

**Don't**:

> Coder supports DevContainers as a workspace runtime.
> (Wrong casing.
> The spec writes the abbreviated form as two words.)
>
> Coder supports Dev Containers as a workspace runtime.
> (Capital-D `Dev Container` is reserved for the proper-noun shorthand `Dev Container Spec`.
> The category is lowercase.)

*Enforced by `Coder.DevContainer` (planned).*

## One term per concept

Pick one name for each thing, then use that name every time the thing appears.
Synonyms read as new concepts.
A page that alternates between "workspace," "environment," and "dev box" makes the reader ask whether the three terms differ.

The [glossary](../../reference/glossary.md) is the registry of canonical names.
When a concept has a glossary entry, use the entry's term.

The same rule covers repeated instructions inside one page.
Word the same action the same way each time it occurs, so the reader recognizes it as the same action.

**Do**:

> Create the workspace from the template.
> When the workspace starts, the agent runs the startup script.

**Don't**:

> Create the workspace from the template.
> When the environment starts, the agent runs the init script.

*Adapted from ASD-STE100 Issue 9, rules 1.11 and 9.4.
Documentation-only.
No Vale rule.*

## Phrasal verbs and their noun forms

English uses two spellings for many product actions: two words when the term is a verb (`set up`, `log in`), and one word (or hyphenated) when the term is a noun (`setup`, `login`).
Treat them consistently across the docs.

| Verb (two words) | Noun (one word or hyphenated) |
|------------------|-------------------------------|
| set up           | setup                         |
| log in           | login                         |
| sign in          | sign-in                       |
| log out          | logout                        |
| back up          | backup                        |
| roll out         | rollout                       |
| start up         | startup                       |
| shut down        | shutdown                      |

`Quickstart` is one word, always, even though it derives from "quick start."

**Do**:

> Follow the Quickstart to set up your first workspace.
>
> The setup takes about 10 minutes.
>
> Log in to Coder, then check that the login appears in the audit log.
>
> Back up the database before the upgrade.
> The backup file lives in `/var/lib/coder/backups`.

**Don't**:

> Follow the Quick Start to setup your first workspace.
>
> The set-up takes about 10 minutes.
>
> Login to Coder, then check that the log in appears in the audit log.
>
> Backup the database before the upgrade.

*Enforced by `Coder.PhrasalVerbs` (planned).*

## Whose for people, not things

"Whose" is the possessive of "who", so it implies the antecedent is a person.
When the antecedent is an inanimate object or an abstract concept, prefer "with", "where", or "that has".

**Do**:

> A chat with pruned gateway records reports no cost.
>
> A template that has conflicting variables fails validation.
>
> A region where every proxy is unhealthy drops workspace connections.

**Don't**:

> A chat whose gateway records have been pruned reports no cost.
>
> A template whose variables conflict fails validation.
>
> A region whose proxies are all unhealthy drops workspace connections.

*Documentation-only.
No Vale rule.*

## Refer to, check out, visit, not see

When the prose points the reader at another page, section, or external resource, choose the verb that matches the register:

- **Refer to** is the formal default for cross-references inside the docs.
  Use it when the destination is a reference page, a specification, or any resource the reader should consult before continuing this doc.
- **Check out** is informal.
  Use it in tutorials and step-by-step passages where the conversational register suits the content.
  Do not use it in reference material.
- **Visit** is best when the destination is an external URL or another site, especially when the reader leaves the docs.

Do not use **see** as a navigational verb.
Reserve **see** for the rare case where the prose describes what a reader observes in the product UI ("You see a list of templates on the Templates page").
The plain-language alternatives carry register information that "see" doesn't, and reserving "see" for its observational meaning improves clarity for every reader.

The same reservation covers "see" used to mean "understand" or "find out".
In a list of outcomes, "learn why the build fails" or "find out why the build fails" names what the reader gains.
"See why the build fails" borrows the observational sense of "see" for a comprehension outcome, so prefer "learn" or "find out."

**Do**:

> For the full command list, refer to the [Coder CLI reference](../../reference/cli/index.md).
>
> Check out the [Quickstart](../../tutorials/index.md) before you configure the production deployment.
>
> Visit the [Terraform Registry](https://registry.terraform.io/) for the latest provider versions.
>
> Add a Ruby option, then learn why the option alone doesn't install the toolchain.

**Don't**:

> For the full command list, see the [Coder CLI reference](../../reference/cli/index.md).
>
> See the [Quickstart](../../tutorials/index.md) before you configure the production deployment.
>
> See the [Terraform Registry](https://registry.terraform.io/) for the latest provider versions.
>
> Add a Ruby option, then see why the option alone doesn't install the toolchain.

*Enforced by `Coder.SeeAlternatives` (planned).*

## Learn more, not Next steps

End-of-page navigation that points the reader at related material uses the heading **Learn more**, not **Next steps**.
The heading choice rests on 2 rationales:

- **Sequencing**: "Next steps" implies the reader must follow a specific sequence.
  "Learn more" frames the section as optional related reading, which matches the Diátaxis distinction between a tutorial (sequenced) and a how-to or reference (independent).
- **Inclusive language**: "steps" reads as a physical-mobility metaphor.
  Readers who can't walk through steps still consume technical documentation.
  Neutral alternatives like "Learn more" don't encode that assumption.

**Do**:

```md
## Learn more

- [Configure SSH access](./ssh.md)
- [Set workspace autostart](./autostart.md)
```

**Don't**:

```md
## Next steps

- [Configure SSH access](./ssh.md)
- [Set workspace autostart](./autostart.md)
```

### The What's next? section in sequenced tutorials

A tutorial in an ordered series may add a **What's next?** section that points to the single next tutorial in that series.
Place it before **Learn more** and write it as a short sentence with the link.

**What's next?** is distinct from **Learn more**: it carries the reader along a defined sequence, while **Learn more** stays optional.
It also avoids the "steps" mobility metaphor, so the ban on **Next steps** still holds.

**Do**:

```markdown
## What's next?

Now that you added a language, [install your own command-line tools](./install-command-line-tools.md).

## Learn more

- [Parameters](../../admin/templates/extending-templates/parameters.md) in the Coder documentation
```

*Enforced by `Coder.LearnMore` (planned).
The planned rule flags **Next steps** only.*

## Tutorial, not walkthrough

`Tutorial` is the standard term in technical documentation and matches the Diátaxis category.
`Walkthrough` is colloquial, and the metaphor assumes the reader can walk.
Neutral alternatives like "tutorial" don't encode that assumption.

**Do**:

> This tutorial shows you how to deploy Coder on AWS.

**Don't**:

> This walkthrough shows you how to deploy Coder on AWS.

*Enforced by `Coder.Tutorial` (planned).*

## Select, not click

Use "select" for actions on UI elements, regardless of input device.
Not every reader clicks: sighted mouse users click, but keyboard-only users tab to a control and press Enter, touch users tap, and screen-reader users activate a control without a pointer at all.
"Click" names the first group's motion and excludes the rest, so it isn't a style preference; a reader with no way to click still needs to know what to do.
"Select" names the outcome instead of the mechanism and covers every input method, matching the Microsoft style guide convention.
Where a more specific verb is clearer, use it instead as long as it doesn't name a device: "open," "expand," "run," and "choose" are all device-agnostic.
This is one instance of the broader [input-device-agnostic language](./accessibility-and-inclusion.md#input-device-agnostic-language) principle.

Not every instance of "click" is a violation. Reserve it for:

- Code or configuration that literally fires on a click event, like an `onClick` handler or a DOM `click` event (already exempt: Vale's prose scope skips code spans and fenced code blocks).
- Explicit mouse-button phrasing: "click," "left-click," "right-click," "middle-click," and "mouse click," with their `-s`/`-ed`/`-ing` forms, describe a literal mouse action and have no device-agnostic equivalent worth writing around.
- "One-click" and "single-click" as a compound feature descriptor ("one-click install," "a single-click button"), and the industry term "ClickOps."

Mouse-button phrasing carries a condition: state the keyboard- or screen-reader-accessible way to do the same thing in the same passage, unless the surrounding content is itself about a specific device (a keyboard shortcut table, a touch-gesture reference, third-party software that records mouse and keyboard input by name).
If no accessible path exists, that's a product gap to raise with engineering, not something to paper over by documenting the mouse-only path as if it were the only one.

**Do**:

> Select **Save** to apply the changes.
>
> Select **Templates** > **Settings** > **Schedule**.
>
> Open the **Templates** tab, then select **Advanced**.
>
> On Mac or Windows, select the files, then open the context menu (right-click, or press the Menu key on Windows) and choose **Compress**.

**Don't**:

> Click **Save** to apply the changes.
>
> Click on the **Templates** tab, then click **Settings**.
>
> On Mac or Windows, highlight the files and right-click; choose **Compress** from the menu that appears.

The third **Don't** example names a mouse action with no accessible alternative given, which the fourth **Do** example fixes by adding the keyboard path.

*Enforced by `Coder.SelectClick` at `warning` severity: a substitution rule that suggests `select` or `open` in place of `click`/`clicks`/`clicked`/`clicking`.*

## Don't assume simplicity or difficulty

Words that minimize the difficulty of an action ("simply," "just," "easy," "easily," "obviously," "of course," "clearly") assume the reader's experience matches the author's.
If something is "obvious" to the author and not to the reader, the reader may feel the document is confusing or condescending.
Cut the simplicity-assuming word or restructure the sentence.

The reverse pattern, exaggerating difficulty ("complex," "intricate," "non-trivial"), is also banned.
Both patterns predict the reader's reaction instead of describing the work.

**Do**:

> Run `coder login` to authenticate.

**Don't**:

> Simply run `coder login` to authenticate.
> It's easy!
>
> The non-trivial process of authenticating with Coder requires running `coder login`.

*Enforced by `Coder.AssumeDifficulty` (planned).*

## Avoid weasel words

Vague attributions ("many believe," "some say," "experts agree," "studies show," "it is widely accepted that," "most people") let the prose claim something without naming a source.
Either name the source or remove the claim.

Vague qualifiers ("often," "usually," "sometimes," "in most cases") tell the reader the statement is sometimes false but don't say when.
Replace with the specific condition, or remove the qualifier and accept the statement as a default.

**Do**:

> The Coder agent reconnects within 30 seconds of a network drop.
>
> The [Coder benchmarks](../../about/why-coder.md) show a 40% reduction in onboarding time for new developers.
>
> The provisioner runs `terraform plan` before `terraform apply`.

**Don't**:

> The Coder agent usually reconnects within a reasonable time.
>
> Many developers believe Coder reduces onboarding time.
>
> Experts agree that running `terraform plan` first is best practice.

*Enforced by `Coder.WeaselWords` (planned).*

## Stop, not kill; turn off, not disable

In product-facing prose, prefer "stop" over "kill" and "turn off" over "disable".
The plain-language forms read better for a non-technical audience and don't carry violent or ableist connotations.

The rule has scoped exceptions for unavoidable industry-specific terms.
When the prose names a specific technical command or a real state label, the original term is the only correct one.
Wrap the term in backticks to signal that the prose is naming a tool or a state, not using the violent verb.

The exceptions are:

- The Linux `kill` command (process control) and the `SIGKILL` signal.
  When the prose tells the reader to terminate a process from a shell, the literal command is `kill <pid>`.
  In prose, write "stop the process" or "end the process" instead.
  Use `kill` in backticks only when the prose names the command itself.
- The `disabled` state of a feature flag in configuration.
  Configuration values keep their literal name (`disabled: true`), and prose describing the flag also uses the state name in backticks.
- The `killed` status of a process in a log file or in CLI output.
  The log line preserves the original wording.

The Coder docs team is aware that the most natural verb for software (`run`) carries similar connotations.
A dedicated rule for `run` is out of scope for this revision.

**Do**:

> To stop a workspace, select **Stop** in the workspace dashboard.
>
> You can turn off auto-update in the template settings.
>
> If the provisioner hangs, end the process from the shell.
> The literal command is `kill <pid>` or `pkill provisionerd`.
>
> The agent reports a `killed` status when the supervisor terminated the process.

**Don't**:

> To kill a workspace, select **Kill** in the workspace dashboard.
>
> You can disable auto-update in the template settings.
>
> If the provisioner hangs, kill the process from the shell.
> (Plain-text `kill` used where backticks are required, and the verb reads as violent.)

*Enforced by `Coder.PlainLanguage` (planned), with the industry-term exception scoped in the rule.*

## Keep internal-only references out of published docs

The published documentation, including the contribution guides, is public.
Every reader and every contributor, whether a community contributor or a Coder employee, must be able to open every resource linked from the docs.
A link that only employees can open excludes community contributors, so it doesn't belong on a published page.

Keep these out of published pages:

- Issue-tracker identifiers and URLs (for example, an `ABC-123` identifier or a `linear.app` link).
- Private or internal-only repositories and their URLs.
- Internal-only chat threads, design docs, dashboards, runbooks, and wikis.
- Any link gated behind employee-only access.

Track the work in the surfaces built for it.
A pull request description, a commit message, or a code-review comment is the right place to cite an internal issue ID or a private link, because every contributor on that change can read it there.
The published page stays the same for everyone.

**Do**:

> The provisioner retries the build 3 times before it fails.

**Don't**:

> The provisioner retries the build 3 times before it fails.
> For the backstory, refer to [ABC-123](https://linear.app/acme/issue/ABC-123).

*Documentation-only.
Planned Vale rule `Coder.InternalReferences`.*

## Learn more

- [Style guide landing page](./README.md)
- [Voice and tone](./voice-and-tone.md)
- [Accessibility and inclusion](./accessibility-and-inclusion.md)
