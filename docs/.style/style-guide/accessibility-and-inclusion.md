# Accessibility and inclusion

The Coder documentation aims for [WCAG 2.1](https://www.w3.org/TR/WCAG21/) Level AA conformance as a minimum, with Level AAA as a stretch goal where it does not sacrifice clarity.
The rules on this page support that target.
They cover heading structure, inclusive language, link text, images, plain English for international readers, page descriptions, and reading level.

> [!NOTE] Color contrast and other rendered-output a11y concerns belong to the docs site theme, not to prose conventions.
> The Coder docs team tracks color-contrast conformance separately.

## Heading structure and placement

Each page has exactly one H1.
The H1 is the page title and appears once at the top of the page.
Subsequent headings descend by one level at a time.
A page goes H1, then H2, then H3.
A page does not jump from H2 to H4.

Each heading is followed by at least one paragraph (or other content block) before the next heading.
A bare H2 followed immediately by an H3 with no prose in between reads as a broken document outline, and SEO crawlers flag the pattern as a potential site error.
If a parent heading does not yet have introductory content, write a short paragraph that frames what the section covers before the subheadings.

The rule is a [WCAG 2.1 Level A](https://www.w3.org/TR/WCAG21/#info-and-relationships) requirement: assistive technology relies on heading levels to convey document structure.
Skipping a level breaks the outline.

**Do**:

```markdown
# Configure your workspace

This page walks through the configuration options exposed on a Coder workspace.
The sections below cover SSH access and environment variables.

## Set up SSH access

SSH access uses the agent that runs inside your workspace.
Two client setups are documented below.

### Connect through JetBrains Toolbox

Install the Coder plugin in JetBrains Toolbox,
then connect to your workspace by name.

### Connect through VS Code Remote SSH

The Coder VS Code extension wraps the standard Remote SSH client and configures it automatically.

## Configure environment variables

Environment variables persist across workspace restarts.
Define them in the template or in the workspace's parameters.
```

**Don't**:

```markdown
# Configure your workspace

# Configure your environment

#### Connect through JetBrains Toolbox

Install the Coder plugin in JetBrains Toolbox,
then connect to your workspace by name.
```

The second H1 creates two competing page titles.
The H1 to H4 jump skips H2 and H3.
Even if the levels were correct, the first H1 has no paragraph before the next heading, which also fails the rule.

*Enforced by `markdownlint` rules `MD001` (heading-increment) and `MD025` (single-h1).
The "content between headings" rule is documentation-only.*

## Inclusive pronouns

Use the singular `they` when the subject's gender is unknown or irrelevant.
Avoid `he or she`, `(s)he`, and similar constructions.

**Do**:

> When a user opens a workspace, they connect to the agent over a Tailscale tunnel.

**Don't**:

> When a user opens a workspace, he or she connects to the agent over a Tailscale tunnel.

*Enforced by `Google.Gender` and `Google.GenderBias`.*

## Inclusive-language substitutions

Use the industry-standard inclusive substitutions for terms that have transitioned across the broader developer-tooling ecosystem.

| Do                                                    | Don't                                         |
|-------------------------------------------------------|-----------------------------------------------|
| allowlist                                             | whitelist                                     |
| blocklist, denylist                                   | blacklist                                     |
| primary, main                                         | master (for the primary branch or controller) |
| primary, hub, reference                               | master (general usage)                        |
| replica, secondary                                    | slave                                         |
| placeholder, sample, mock                             | dummy                                         |
| smoke testing, confidence testing, acceptance testing | sanity check, sanity test                     |

*Enforced by `Coder.InclusiveLanguage` (planned), with additional coverage from the curated `alex.*` lexicon.*

## Descriptive link text

Link text describes what the reader gets at the destination.
Generic phrases like "click here" and "this link" tell the reader nothing if they scan the link out of context.
Screen readers announce link text out of context too, which is the [WCAG 2.1 Level A](https://www.w3.org/TR/WCAG21/#link-purpose-in-context) requirement the rule supports.

**Do**:

> Refer to the [Coder CLI reference](../../reference/cli/index.md) for the full command list.

**Don't**:

> Refer to the Coder CLI reference [here](../../reference/cli/index.md).
>
> [Click here](../../reference/cli/index.md) for the full command list.

*Enforced by `Coder.LinkText` (planned).*

## Alt text for images

Every image declares descriptive alt text.
The alt text describes what the image shows or what purpose it serves.
It is not a caption.
Captions go below the image in a `<small>` tag.

Aim for one or two sentences that convey the same information a sighted reader would extract from the image.
Lead with the subject, not "An image of" or "A screenshot showing".

```markdown
![Template Insights dashboard with weekly active users and connection latency charts](../../images/admin/templates/template-insights.png)

<small>The Template Insights dashboard. Active users in the left panel; connection latency in the right panel.</small>
```

For complex diagrams that cannot be summarized in alt text, provide a longer description in the body of the page and reference it from the alt text.

*Enforced by `markdownlint` rule `MD045` for the alt-text-required requirement.*

## Decorative images

Mark images that carry no information beyond visual decoration with empty alt text.
Empty alt text tells the screen reader to skip the image rather than announce a meaningless filename.

```markdown
![](../../images/decorative/divider.png)
```

Decorative images are rare in the Coder docs.
Most images shown to a reader are screenshots or diagrams that convey information, and those images need descriptive alt text.
When in doubt, write descriptive alt text.

*Documentation-only.
No Vale rule.*

## Plain English for international readers

Keep prose accessible to readers whose first language is not English.
Two patterns add friction for non-native speakers without adding meaning, so the guide bans them:

### Avoid idioms and figurative language

Idioms (`under the weather`, `ballpark figure`, `get the ball rolling`, `at the eleventh hour`) and figurative language (`unleash`, `supercharge`, `dive in`, `out of the box`) rely on cultural context that does not translate.
They also rarely add precision.
Replace them with the literal meaning.

**Do**:

> The estimated startup time is between 30 and 60 seconds.
>
> Run `coder login` to begin.
>
> Coder ships with a default template.

**Don't**:

> The ballpark figure for startup time is 30 to 60 seconds.
>
> Run `coder login` to get the ball rolling.
>
> Coder ships with a default template out of the box.

*Documentation-only.
Planned Vale rule `Coder.Idioms`.*

### Latin abbreviations

The following Latin abbreviations are fine in Coder docs.
Use them when they fit the sentence; the English equivalent is also fine.

| Abbreviation | Meaning                                        | Notes                                                                          |
|--------------|------------------------------------------------|--------------------------------------------------------------------------------|
| `e.g.`       | for example                                    | Followed by a comma. Prefer parentheses around the clause, as described below. |
| `i.e.`       | that is                                        | Followed by a comma. Prefer parentheses around the clause, as described below. |
| `etc.`       | and so on                                      | Closes a list. The Oxford comma applies before it: `apples, oranges, etc.`     |
| `vs.`        | versus, against, as opposed to, in contrast to | No comma. Example: `coder server vs. coder agent`.                             |
| `et al.`     | and others                                     | Citation contexts only. Follow the citation style's punctuation rules.         |

**Prefer parentheses around `e.g.` and `i.e.` clauses.** The parentheses make the sentence structure obvious and avoid a cascade of commas around the abbreviation.

**Do**:

> Many compute platforms work (e.g., AWS, GCP, or a self-managed Kubernetes cluster).
>
> The agent exits when the workspace stops (i.e., when the build phase tears down).

**Don't**:

> Many compute platforms work, e.g., AWS, GCP, or a self-managed Kubernetes cluster.
>
> The agent exits when the workspace stops, i.e., when the build phase tears down.

The **Don't** versions are grammatical, but the comma cascade makes the sentence structure harder to follow.

**One period when `etc.` ends a sentence.** The period in `etc.` doubles as the sentence-ending period.

**Do**:

> The provisioner installs apples, oranges, etc.

**Don't**:

> The provisioner installs apples, oranges, etc..

When `etc.` ends a parenthetical at the end of a sentence, keep both periods.
The abbreviation's period closes `etc.`, the closing parenthesis follows, and the sentence-ending period falls outside the parenthesis.

**Do**:

> The provisioner handles produce (apples, oranges, etc.).

The same rule applies if `e.g.` or `i.e.` ever sits at the end of a sentence, though that placement is unusual.

**Citation form for `et al.`** In an author-date citation, place a comma between the author phrase and the year, and keep the abbreviation's period.

**Do**:

> The protocol is described by Smith et al., 2020.
>
> The protocol is described by Smith et al. (2020).

**Less common Latin abbreviations are not allowed.** Latin abbreviations beyond the five in the table, such as `a priori`, `q.v.`, `viz.`, `n.b.`, `cf.`, and `ibid.`, are unfamiliar to many readers and easy to misuse.
Replace them with plain English.

**Don't**:

> The default configuration is acceptable a priori.
>
> Refer to the deployment guide, q.v. for benchmarks.

<details>
<summary>Why these specific abbreviations are allowed</summary>

Major plain-language guides such as the [Google developer documentation style guide](https://developers.google.com/style/abbreviations), the [Microsoft Writing Style Guide](https://learn.microsoft.com/en-us/style-guide/abbreviations/), the [18F Content Guide](https://content-guide.18f.gov/our-style/inclusive-language/), and the [Plain Language Action and Information Network (PLAIN) federal guidance](https://www.plainlanguage.gov/guidelines/words/use-simple-words-phrases/) recommend English equivalents for all Latin abbreviations.
The argument is that the abbreviations are unfamiliar to many readers and frequently misused (`i.e.` confused with `e.g.`).

The Coder docs follow the spirit of that guidance for less common Latin but make an exception for `e.g.`, `i.e.`, `etc.`, `vs.`, and `et al.` These five are near-universal in industry technical writing; restricting them adds friction for writers without a clear payoff for readers familiar with the conventions of the genre.

</details>

*Documentation-only.
No Vale rule.*

## Page title and sidebar title

A page's H1 and its sidebar title serve different jobs and may diverge.

- The **H1** is the page's grammatical declaration of what the page does.
  It works as the only line of text when the page is opened in isolation (Markdown source, RSS feed, mobile view that hides the sidebar, or a permalink shared in chat).
  Refer to [Declare audience and scope up front](./audience-and-scope.md#declare-audience-and-scope-up-front) for how the H1 names the outcome.
- The **sidebar title** is a navigation label.
  It fits the limited horizontal space of the sidebar and reads fast when the reader is scanning a tree of dozens of pages.
  The Coder docs site reads the sidebar title from the `title` field in [`docs/manifest.json`](../../manifest.json).

The two must each stand alone, but they do not need to be identical.
Breadcrumb depth gives one layer of context for free.
The sidebar title can drop redundancy that the parent breadcrumbs already imply.

Worked example.
A page reachable through **Administration** > **Authentication** > **Google** has parent breadcrumbs that already say "Administration" and "Authentication".
The sidebar title can be `Google` alone, and the H1 can be `Configure Google authentication with Coder`.
Both labels stand alone in their own context.

When the H1 and the sidebar title coincide (often the case for short-titled pages), that is fine.
When they diverge, the divergence is intentional and serves the reader.
The same pattern is common in mature docs sites.
AWS, Microsoft Learn, and GitHub Docs all pair task-focused H1s with shorter noun-focused sidebar titles.

**Do**:

| Sidebar title | H1                                             |
|---------------|------------------------------------------------|
| Google        | Configure Google authentication with Coder     |
| Helm chart    | Deploy Coder on Kubernetes with the Helm chart |
| OIDC          | Configure single sign-on with OIDC             |

**Don't**:

| Sidebar title                              | H1                                             |
|--------------------------------------------|------------------------------------------------|
| Configure Google authentication with Coder | Configure Google authentication with Coder     |
| Click here for Helm install                | Deploy Coder on Kubernetes with the Helm chart |
| Page                                       | Configure single sign-on with OIDC             |

The first **Don't** row uses the full H1 as the sidebar title.
The sidebar title is redundant with the parent breadcrumbs and crowds the navigation tree.
The second row has a sidebar title that does not stand alone.
The third row has a sidebar title that tells the reader nothing.

*Documentation-only.
No Vale rule.*

## Page descriptions

Each page declares a description that appears in search engine results, in social-media previews, and in screen-reader page summaries.
The Coder docs site reads descriptions from [`docs/manifest.json`](../../manifest.json), not from YAML front matter inside the Markdown file.
The manifest maps each page to a `title` and a `description`:

```json
{
  "title": "Configure your workspace",
  "description": "Configure SSH access, environment variables, and autostart for a Coder workspace.",
  "path": "./admin/workspaces/configure.md"
}
```

A good description:

- States what the page covers in one sentence.
- Stays under roughly 160 characters so search engines do not truncate it.
- Avoids marketing language and superlatives.
- Reads as a complete sentence.

**Do**:

```json
"description": "Configure SSH access, environment variables, and autostart for a Coder workspace."
```

**Don't**:

```json
"description": "Workspace configuration"
```

```json
"description": "The best, fastest, most reliable way to configure everything you need to know about Coder workspaces."
```

The short description tells the reader nothing.
The marketing description does not survive truncation and adds no information.

If a page does not yet have a description in the manifest, add one in the same PR that touches the page.

*Documentation-only.
No Vale rule.*

## Reading level

Aim for a Flesch-Kincaid grade level of 8 to 10 in body prose.
The target supports comprehension for non-native English readers, ESL audiences, and anyone skimming under time pressure.
The reading-level rule decomposes into prose rules covered elsewhere in this guide:

- Short sentences.
  Aim for 25 words or fewer.
- [Active voice by default](./voice-and-tone.md#active-voice-by-default).
- [Present tense by default](./voice-and-tone.md#present-tense-by-default).
- Common words.
  Define jargon on first use.
- [Plain English for international readers](#plain-english-for-international-readers).
- [Plain language for product actions](./word-choice.md#stop-not-kill-turn-off-not-disable).
- [No weasel words](./word-choice.md#avoid-weasel-words).

A reading-level rule is part of [WCAG 2.1 Level AAA](https://www.w3.org/TR/WCAG21/#reading-level) Success Criterion 3.1.5.
The criterion is satisfied either by writing at the lower-secondary reading level or by providing an alternative version.
Coder docs write at the target reading level directly.

Editors that surface a grade-level score (Hemingway, Vale's `write-good.Reading`) are a useful spot check.
The grade level is not a hard ceiling.
A reference page that requires technical vocabulary will read higher than a tutorial, and that is correct.

*Documentation-only.
No Vale rule wired.*

## Color contrast

The docs site theme controls color contrast, not the prose written on each page.
Tracked separately from this guide.
The target is WCAG 2.1 Level AA for normal text (contrast ratio 4.5:1) and Level AA for large text (3:1), with AAA (7:1 normal, 4.5:1 large) as the stretch goal.

*Out of scope for this guide.
Tracked by the docs site theme.*

## Related

- [Style guide landing page](./README.md)
- [Voice and tone](./voice-and-tone.md)
- [Word choice](./word-choice.md)
- [Formatting](./formatting.md)
