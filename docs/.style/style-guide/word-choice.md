# Word choice

Coder documentation uses canonical brand and product names,
plain language for product actions,
and "refer to" instead of "see" for navigational pointers.
The rules on this page set those defaults.

For inclusive-language substitutions like `allowlist` or `primary`,
refer to [Accessibility and inclusion](./accessibility-and-inclusion.md).

## Coder product and feature names

`Coder`, the company and the product, is always capitalized.
Feature names are capitalized as proper nouns when the prose names the feature.
The underlying generic concept stays lowercase.

When the prose refers to the Coder command-line interface as a tool,
wrap it in backticks: `coder`.
The bare lowercase `coder` (no backticks) is wrong.
It reads as a misspelling of the product name.

| Do                              | Don't                                          |
|---------------------------------|------------------------------------------------|
| Coder                           | coder (referring to the product, no backticks) |
| `coder` (the CLI, in backticks) | coder (the CLI, no backticks)                  |
| AI Bridge                       | AI bridge, AIBridge                            |
| Workspace Proxy                 | workspace proxy (referring to the feature)     |
| workspace                       | Workspace (referring to the generic concept)   |
| template                        | Template (referring to the generic concept)    |
| agent                           | Agent (referring to the generic concept)       |
| provisioner                     | Provisioner (referring to the generic concept) |

**Do**:

> Coder runs `coder login` to authenticate against the Coder server.
>
> Open the AI Bridge integration page to configure model providers.

**Don't**:

> coder runs coder login to authenticate against the coder server.
>
> Open the ai bridge integration page to configure model providers.

*Enforced by `Coder.ProductTerms` (planned).*

## Brand names

Use the canonical casing for third-party brand and product names.
The Coder docs team keeps a substitution list.

When the prose refers to a third-party command-line tool,
wrap the tool name in backticks the same way as for the Coder CLI.
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

Lowercase forms remain correct in code blocks, URLs, package names, and Terraform provider sources,
where the canonical form is lowercase by convention.

*Enforced by `Coder.BrandNames`.*

## Dev Container terminology

A development container that follows the [Dev Container specification](https://containers.dev/) follows two casings depending on context:

- **`Dev Container`** (capitalized, two words) when the prose names the specification,
  the feature category,
  or the proper noun.
- **`dev container`** (lowercase, two words) when the prose refers to an instance or uses the term as a generic noun.

The rule parallels the `Coder` versus `workspace` distinction:
the proper noun is capitalized,
specific instances are not.

`envbuilder` is the implementation tool Coder uses to build dev containers.
It is not itself the concept,
so it stays in backticks as a tool name.

**Do**:

> Coder supports Dev Containers as a workspace runtime.
> The template defines the Dev Container in a `devcontainer.json` file.
> The provisioner builds the dev container with `envbuilder` and starts the agent inside it.

**Don't**:

> Coder supports dev containers as a workspace runtime.
> (Generic noun used where the proper noun is meant.)
>
> Coder supports DevContainers as a workspace runtime.
> (Wrong casing.)
>
> The provisioner builds the Dev Container with envbuilder.
> (Tool name not in backticks; instance capitalized.)

*Enforced by `Coder.DevContainer` (planned).*

## Phrasal verbs and their noun forms

English uses two spellings for many product actions:
two words when the term is a verb (`set up`, `log in`),
and one word (or hyphenated) when the term is a noun (`setup`, `login`).
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

`Quickstart` is one word, always,
even though it derives from "quick start".

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

## Refer to, check out, visit, not see

When the prose points the reader at another page, section, or external resource,
choose the verb that matches the register:

- **Refer to** is the formal default for cross-references inside the docs.
  Use it when the destination is a reference page, a specification,
  or any resource the reader should consult before continuing this doc.
- **Check out** is informal.
  Use it in tutorials and step-by-step passages where the conversational register suits the content.
  Do not use it in reference material.
- **Visit** is best when the destination is an external URL or another site,
  especially when the reader leaves the docs.

Do not use **see** as a navigational verb.
Reserve **see** for the rare case where the prose describes what a reader observes in the product UI ("You see a list of templates on the Templates page").
The plain-language alternatives carry register information that "see" does not,
and reserving "see" for its observational meaning improves clarity for every reader.

**Do**:

> For the full command list,
> refer to the [Coder CLI reference](../../reference/cli/index.md).
>
> Check out the [Quickstart](../../tutorials/index.md) before you configure the production deployment.
>
> Visit the [Terraform Registry](https://registry.terraform.io/) for the latest provider versions.

**Don't**:

> For the full command list,
> see the [Coder CLI reference](../../reference/cli/index.md).
>
> See the [Quickstart](../../tutorials/index.md) before you configure the production deployment.
>
> See the [Terraform Registry](https://registry.terraform.io/) for the latest provider versions.

*Enforced by `Coder.SeeAlternatives` (planned).*

## Learn more, not Next steps

End-of-page navigation that points the reader at related material uses the heading **Learn more**, not **Next steps**.
Two rationales apply:

- **Sequencing**: "Next steps" implies the reader must follow a specific sequence.
  "Learn more" frames the section as optional related reading,
  which matches the Diátaxis distinction between a tutorial (sequenced) and a how-to or reference (independent).
- **Inclusive language**: "steps" reads as a physical-mobility metaphor.
  Readers who cannot walk through steps still consume technical documentation.
  Neutral alternatives like "Learn more" do not encode that assumption.

**Do**:

```markdown
## Learn more

- [Configure SSH access](./ssh.md)
- [Set workspace autostart](./autostart.md)
```

**Don't**:

```markdown
## Next steps

- [Configure SSH access](./ssh.md)
- [Set workspace autostart](./autostart.md)
```

*Enforced by `Coder.LearnMore` (planned).*

## Tutorial, not walkthrough

`Tutorial` is the standard term in technical documentation and matches the Diátaxis category.
`Walkthrough` is colloquial,
and the metaphor assumes the reader can walk.
Neutral alternatives like "tutorial" do not encode that assumption.

**Do**:

> This tutorial shows you how to deploy Coder on AWS.

**Don't**:

> This walkthrough shows you how to deploy Coder on AWS.

*Enforced by `Coder.Tutorial` (planned).*

## Select, not click

Use "select" for actions on UI elements,
regardless of input device.
"Click" assumes a mouse.
Touch devices tap,
keyboard users press Enter,
and assistive-technology users activate.
"Select" covers every case and matches the Microsoft style guide convention.

Reserve "click" for code or configuration that literally fires on a click event,
like a `onClick` handler or a DOM `click` event.

**Do**:

> Select **Save** to apply the changes.
>
> Select **Templates** > **Settings** > **Schedule**.

**Don't**:

> Click **Save** to apply the changes.
>
> Click on the **Templates** tab,
> then click **Settings**.

*Enforced by `Coder.SelectClick` (planned).*

## Don't assume simplicity or difficulty

Words that minimize the difficulty of an action ("simply", "just", "easy", "easily", "obviously", "of course", "clearly") assume the reader's experience matches the author's.
If something is "obvious" to the author and not to the reader,
the reader may feel the document is confusing or condescending.
Cut the simplicity-assuming word or restructure the sentence.

The reverse pattern, exaggerating difficulty ("complex", "intricate", "non-trivial"), is also banned.
Both patterns predict the reader's reaction instead of describing the work.

**Do**:

> Run `coder login` to authenticate.

**Don't**:

> Simply run `coder login` to authenticate. It's easy!
>
> The non-trivial process of authenticating with Coder requires running `coder login`.

*Enforced by `Coder.AssumeDifficulty` (planned).*

## Avoid weasel words

Vague attributions ("many believe", "some say", "experts agree", "studies show", "it is widely accepted that", "most people") let the prose claim something without naming a source.
Either name the source or remove the claim.

Vague qualifiers ("often", "usually", "sometimes", "in most cases") tell the reader the statement is sometimes false but do not say when.
Replace with the specific condition,
or remove the qualifier and accept the statement as a default.

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

In product-facing prose,
prefer "stop" over "kill" and "turn off" over "disable".
The plain-language forms read better for a non-technical audience and do not carry violent or ableist connotations.

The rule has scoped exceptions for unavoidable industry-specific terms.
When the prose names a specific technical command or a real state label,
the original term is the only correct one.
Wrap the term in backticks to signal that the prose is naming a tool or a state,
not using the violent verb.

The exceptions are:

- The Linux `kill` command (process control) and the `SIGKILL` signal.
  When the prose tells the reader to terminate a process from a shell,
  the literal command is `kill <pid>`.
  In prose, write "stop the process" or "end the process" instead.
  Use `kill` in backticks only when the prose names the command itself.
- The `disabled` state of a feature flag in configuration.
  Configuration values keep their literal name (`disabled: true`),
  and prose describing the flag also uses the state name in backticks.
- The `killed` status of a process in a log file or in CLI output.
  The log line preserves the original wording.

The Coder docs team is aware that the most natural verb for software (`run`) carries similar connotations.
A dedicated rule for `run` is out of scope for this revision.

**Do**:

> To stop a workspace,
> select **Stop** in the workspace dashboard.
>
> You can turn off auto-update in the template settings.
>
> If the provisioner hangs, end the process from the shell.
> The literal command is `kill <pid>` or `pkill provisionerd`.
>
> The agent reports a `killed` status when the supervisor terminated the process.

**Don't**:

> To kill a workspace,
> select **Kill** in the workspace dashboard.
>
> You can disable auto-update in the template settings.
>
> If the provisioner hangs, kill the process from the shell.
> (Plain-text `kill` used where backticks are required, and the verb reads as violent.)

*Enforced by `Coder.PlainLanguage` (planned),
with the industry-term exception scoped in the rule.*

## Related

- [Style guide landing page](./README.md)
- [Voice and tone](./voice-and-tone.md)
- [Accessibility and inclusion](./accessibility-and-inclusion.md)
