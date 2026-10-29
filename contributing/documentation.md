# Documentation

This style guide is primarily for use with authoring documentation.

## General guidelines

- Use sentence case, even in titles (do not punctuate the title, though)
- Use the second person
- Use the active voice
- Use plural nouns and pronouns (_they_, _their_, or _them_), especially when
  the specific number is uncertain (i.e., "Set up your environments" even though
  you don't know if the user will have one or many environments)
- When writing documentation titles, use the noun form, not the gerund form
  (e.g., "Environment Management" instead of "Managing Environments")
- Context matters when you decide whether to capitalize something or not. For
  example,
  ["A Job creates one or more Pods..."](https://kubernetes.io/docs/concepts/workloads/controllers/job/)
  is correct when writing about Kubernetes. However, in other contexts, neither
  _job_ nor _pods_ would be capitalized. Please follow the conventions set forth
  by the relevant companies and open source communities.

## Third-party references

If you have questions that aren't explicitly covered by this guide, consult the
following third-party references:

| **Type of guidance** | **Third-party reference**                                                              |
|----------------------|----------------------------------------------------------------------------------------|
| Spelling             | [Merriam-Webster.com](https://www.merriam-webster.com/)                                |
| Style - nontechnical | [The Chicago Manual of Style](https://www.chicagomanualofstyle.org/home.html)          |
| Style - technical    | [Microsoft Writing Style Guide](https://docs.microsoft.com/en-us/style-guide/welcome/) |

## Tools

This repository runs [Vale](https://vale.sh/) on `docs/` as part of CI.
The configuration is the repo-root `.vale.ini`, and the curated rule set is documented in [`docs/.style/style-guide/`](https://github.com/coder/coder/tree/main/docs/.style/style-guide).
Run the same checks locally with `make lint/prose`.

Vale checks a small subset of the style guide, and it runs in advisory mode: warnings surface as inline PR annotations, but they don't block the build.
A clean Vale run doesn't mean a page follows the guide.
Refer to [What the tooling checks, and what it doesn't](https://github.com/coder/coder/blob/main/docs/.style/style-guide/README.md#what-the-tooling-checks-and-what-it-doesnt) for the current coverage, then apply the guide yourself.

The following external tools can also help when drafting. Take their
suggestions with a grain of salt because they aren't tuned for Coder's
voice or product names:

- [alex.js](https://alexjs.com/)
- [Grammarly](https://app.grammarly.com/)
- [Hemingway Editor](https://hemingwayapp.com/)

## Feature states

A feature state, such as `premium`, `beta`, or `early access`, marks a page in
the sidebar and under the page title.
Set it with the `state` key on the page's route in the YAML sidebar sources
under [`docs/manifest/`](../docs/manifest/), not in the page title or H1.

A state on a route applies to that route and to every route beneath it.
Follow these rules:

1. Put the state on the highest route it applies to.
   For a Premium feature with several pages, set `state: [premium]` on the
   feature's top-level route only.
2. Do not repeat an ancestor's state on a descendant.
   The descendant inherits it.
3. Add a state to a descendant only when the descendant has a state that its
   ancestors lack, such as an early access subfeature inside a Premium feature.
   List only the new state; the inherited one still applies.

`make lint/docs-manifest` fails when a route repeats a state that an ancestor
already sets, and names the file, the route, and the ancestor.
Include files count: an ancestor in another file still counts as an ancestor.

### Example

A Premium feature with an early access subfeature:

```yaml
# docs/manifest/admin/widgets.yml
title: Widgets
description: Manage widgets across your deployment.
path: ./admin/widgets/index.md
state: [premium]
children:
  - title: Widget setup
    description: Enable and configure widgets.
    path: ./admin/widgets/setup.md
  - title: Widget sync
    description: Sync widgets between deployments.
    path: ./admin/widgets/sync.md
    state: [early access]
```

`Widget setup` sets no state and inherits Premium.
`Widget sync` sets only `early access` and inherits Premium as well.
Do not write `state: [premium, early access]` on `Widget sync`, because the lint
rejects the repeated `premium`.

The docs site shows each page as follows:

| Route          | Own `state`      | Sidebar pill | Pills under the page title |
|----------------|------------------|--------------|----------------------------|
| `Widgets`      | `[premium]`      | Premium      | Premium                    |
| `Widget setup` | none             | none         | Premium                    |
| `Widget sync`  | `[early access]` | Early access | Premium, Early access      |

The sidebar shows a pill on the route that sets a state and on any descendant
whose state differs, so a section reads as Premium once instead of on every
entry.
The pills under a page title show the page's full state: its own states plus
every state it inherits.

Inherited pills under the page title require the docs-engine change that adds
state inheritance.
Until the docs site runs that version, a page shows only the states set on its
own route.

Refer to the feature states rule in the
[content guidelines](../docs/.style/content-guidelines.md#structural-rules)
for the canonical rule.

## How to format text

Below summarizes the text-formatting conventions you should follow.

### Bold

Use **bold** formatting when referring to UI elements.

### Italics

Use _italics_ for:

- Parameter names
- Mathematical and version variables

### Code font

Use _code font_ for:

- User text input
- Command-line utility names
- DNS record types
- Environment variable names (e.g., `PATH`)
- Filenames, filename extensions, and paths
- Folders and directories
- HTTP verbs, status codes, and content-type values
- Placeholder variables (see [Placeholders in angle brackets](#placeholders-in-angle-brackets))

Use _code blocks_ for code samples and other blocks of code. Be sure to indicate
the language your using to apply the proper syntax highlighting.

```txt
This is a codeblock.
```

For code that you want users to enter via a command-line interface, use
`console`, not `bash`.

### Placeholders in angle brackets

Placeholder values written in angle brackets **must** be wrapped in backticks so
they render as inline code. The documentation site renders Markdown as HTML, so
an unwrapped placeholder such as `<organization-name>` is parsed as an unknown
HTML tag and stripped out of the page, so readers never see it.

This applies wherever the text becomes documentation, including the Go CLI help
strings and Swagger annotations that generate the CLI and API reference.

```text
# Correct: wrap the placeholder in backticks
Structured as: `<organization-name>/<team-slug>`.

# Incorrect: the placeholder renders as "Structured as: /."
Structured as: <organization-name>/<team-slug>.
```

### Punctuation

Do not use the ampersand (&) as a shorthand for _and_ unless you're referring to
a UI element or the name of something that uses _&_.

You can use the symbol `~` in place of the word _approximately_.

### UI elements

When referring to UI elements, including the names for buttons, menus, dialogs,
and anything that has a name visible to the user, use bold font.

**Example:** On the **Environment Overview** page, select **Configure SSH**.

Don't use code font for UI elements unless it is rendered based on previously
entered text. For example, if you tell the user to provide the environment name
as `myEnvironment`, then use both bold and cold font when referring to the name.

**Example**: Select **`myEnvironment`**.

When writing out instructions that involve UI elements, both of the following
options are acceptable:

- Go to **Manage** > **Users**.
- In the **Manage** menu, select **Users**.

## Product-specific references

Below summarizes the guidelines regarding how Coder terms should be used.

### Capitalized terms

The only Coder-specific terms that should be capitalized are the names of
products (e.g., Coder).

The exception is **code-server**, which is always lowercase. If it appears at
the beginning of the sentence, rewrite the sentence to avoid this usage.

### Uncapitalized terms

In general, we do not capitalize the names of features (unless the situation
calls for it, such as the word appearing at the beginning of a sentence):

- account dormancy
- audit logs
- autostart
- command-line interface
- dev URLs
- environment
- image
- metrics
- organizations
- progressive web app
- registries
- single sign-on
- telemetry
- workspace
- workspace providers
- workspaces as code

We also do not capitalize the names of user roles:

- auditor
- member
- site admin
- site manager

## Standardized spellings

- WiFi
