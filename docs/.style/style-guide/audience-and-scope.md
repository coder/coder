# Audience and scope

Every page in the Coder documentation targets one audience working toward one outcome.
The audience determines vocabulary, depth, and the prior knowledge the page assumes.
The outcome determines what the page covers and where it stops.

Pages that try to serve two audiences, or chain multiple unrelated outcomes, serve none of their readers well.
A reader who is one persona away from the page's target has to skip past content that does not apply to them, guess which sentences are for them, and trust the writer not to have buried a step they need inside a section labeled for someone else.

The single canonical Coder example is **install Coder**.
An end user wants to connect their local editor to a Coder workspace and start coding.
A platform engineer wants to deploy the Coder control plane to their company's Kubernetes cluster.
Both groups search for "install Coder.
A page that tries to cover both forces the end user to read past Helm chart values, and forces the platform engineer to read past Visual Studio Code download links.
Two pages, one per audience and one per outcome, serve both groups better than one page that combines them.

## Pick one audience per page

Choose the audience before you choose the words.
The audience determines:

- The product vocabulary the reader already knows (for example, whether `workspace` needs a definition).
- The infrastructure context the reader brings (for example, whether Kubernetes is assumed).
- The level of depth the reader expects (overview, how-to, reference, or in-depth tutorial).

If a topic genuinely needs to serve two audiences, write two pages and cross-link them.
Resist the temptation to write one page with audience-tagged sections.
Section tags do not save readers from scanning content that does not apply to them.

**Do**:

```markdown
# Connect Visual Studio Code to your Coder workspace

*Audience: a developer with an existing Coder workspace.*

This page covers the Visual Studio Code IDE.
For Cursor, refer to [Cursor](./cursor.md).
For Windsurf, refer to [Windsurf](./windsurf.md).
```

**Don't**:

```markdown
# Connect to your Coder workspace

This page covers Visual Studio Code, Cursor, Windsurf, JetBrains, Vim, the web terminal, and SSH.
Operators provisioning the workspace template should refer to the section below on template configuration.
```

## Pick one outcome per page

The outcome is the specific task, or the small set of related tasks, the page helps the reader accomplish.
A how-to page covers one task.
A tutorial covers one chained workflow.
A reference page covers one stable surface (one CLI command, one API endpoint, one schema).
An overview page introduces one concept.

If the page has more than one outcome, split it.
"Configure SSO with Okta" is one outcome.
"Configure SSO" is not.
"Deploy Coder on AWS" is one outcome.
"Deploy Coder" is not.

A page that helps the reader accomplish two unrelated outcomes hides each outcome from the readers who need the other.

**Do**:

```markdown
# Configure single sign-on with Okta

This page walks through configuring OIDC single sign-on against an Okta tenant.
For Azure Active Directory, refer to [Configure SSO with Azure AD](./sso-azure-ad.md).
For Google Workspace, refer to [Configure SSO with Google Workspace](./sso-google.md).
```

**Don't**:

```markdown
# Authentication

This page covers OIDC providers (Okta, Azure AD, Google Workspace, generic OIDC),
SAML providers, GitHub OAuth, password authentication, and the API token model.
```

## Hub pages and category landing pages

Some pages exist to orient the reader and route them to the child page that owns the actual content.
A hub page may have a broad title and a short body when its job is to direct the reader to a child page, not to teach.

Hub pages are not an exemption from the audience and outcome rules.
The audience is the reader looking for the right child page.
The outcome is making the routing decision in three or four lines.

A hub page is appropriate when:

- The topic has several distinct sub-topics.
- Each sub-topic deserves its own page for scope reasons.
- A reader entering the section needs to choose between them.

**Do**:

```markdown
# Authentication

This page is the entry point for configuring authentication in Coder.
Pick the provider that matches your identity source:

- [OpenID Connect (OIDC)](./oidc.md), for Okta, Auth0, Azure AD, Google Workspace, and other OIDC providers.
- [SAML](./saml.md), for SAML 2.0 identity providers.
- [GitHub OAuth](./github-oauth.md), for GitHub-hosted teams.
- [Password authentication](./password.md), for self-hosted local accounts.
```

**Don't**:

```markdown
# Authentication

This page covers OIDC, SAML, GitHub OAuth, password authentication, and the API token model.

## OIDC

[300 lines of provider-specific configuration]

## SAML

[300 lines of provider-specific configuration]
```

The Don't example forces every reader to scan a wall of content for the section that applies to them.
The Do example routes them to the right page in four lines.

A hub page does not need every link to be a direct child page in the file tree.
Cross-references to sibling sections of the docs are valid when that is where the reader's next step lives.

## Declare audience and scope up front

The first paragraph of the page names the audience and the outcome.
The reader should know within the first two or three sentences whether the page is for them and whether it covers their task.

Conventions:

- The H1 names the outcome.
- The first paragraph names the audience and confirms the outcome.
- The first paragraph also links to sibling pages for adjacent audiences or outcomes when those exist.

Do not put a metadata line such as `*Audience: a developer.*` above the first paragraph.
The audience appears in the prose itself.
Do not use the [persona names](#personas-the-coder-docs-serve) inside the page body either.
Persona names are vocabulary for writers planning the page, not for readers reading it.
Name the audience by the role the reader recognizes from their own work (`developer`, `template author`, `Coder deployment administrator`, `organization owner`).

**Do**:

```markdown
# Connect Visual Studio Code to your Coder workspace

This guide is for a developer with an existing Coder workspace.
It covers the Visual Studio Code IDE.
For Cursor, refer to [Cursor](./cursor.md).
For Windsurf, refer to [Windsurf](./windsurf.md).
```

**Don't**:

```markdown
# Kubernetes

Coder runs on Kubernetes.
This page covers many topics related to running Coder on Kubernetes.
```

The Don't title does not name an outcome.
The body does not name an audience.
If the page is a hub that routes the reader, use the pattern in [Hub pages and category landing pages](#hub-pages-and-category-landing-pages).
If the page teaches a single outcome, rename the title and rewrite the opening paragraph.

### Gate privileged pages with a prerequisite callout

Some pages walk through steps that only one role should run.
If a reader from the wrong role follows the steps, they may misconfigure the deployment, escalate their own permissions, or break something for everyone else.

For pages of that kind, add an `IMPORTANT` callout at the top of the page that names the required role and tells the wrong-role reader who to ask.

**Do**:

```markdown
# Configure single sign-on with Okta

This guide is for a Coder deployment administrator
who has access to both the Coder control plane and the Okta tenant.

> [!IMPORTANT]
> You must be a Coder deployment administrator to complete this guide.
> If you are not a deployment administrator,
> ask your administrator to complete the steps for you.
```

The prerequisite callout uses the role the reader recognizes (`Coder deployment administrator`), not the writer-facing persona name (`Perry the Platform Engineer`).

## Give the audience only what it needs

Choosing the audience is also choosing what to leave out.
A page written for a known audience gives that reader what they need to reach the outcome, and nothing that belongs to a different audience.

Knowing the audience means knowing what that audience can already do.
When the page assumes a reader who runs their own Coder deployment, that reader is their own administrator.
Do not hedge a step with "ask your administrator" or "if you have permission".
Those caveats are written for a reader this page does not target, and they make the real reader doubt whether the step is meant for them.

Before you add a caveat, a permission note, or an "if you don't have access" aside, check it against the audience and the full context of the page:

- Does the reader this page targets actually hit this limitation?
- Has the page already established that this reader has the access?
- Does the caveat help this reader, or only a reader who belongs on a different page?

If the caveat serves a different audience, cut it, or move it to the page that audience reads.

**Do** (a local-first Quickstart, where the reader started the server two pages earlier):

> Configure a GitHub provider on your deployment, then create the workspace again.

**Don't**:

> Configure a GitHub provider on your deployment.
> If you are not a deployment administrator, ask your administrator to do this for you.

The **Don't** aside is correct on an enterprise how-to page, where the reader may not own the deployment.
On a Quickstart that walked the same reader through starting the server, the reader already has the access, so the aside only adds doubt.

A page may assume a persona, as long as it knows which persona it assumes and matches its depth and its caveats to what that persona can already do.
This is the complement of [gating privileged pages](#gate-privileged-pages-with-a-prerequisite-callout): add a prerequisite callout when the reader might be the wrong role, and cut wrong-role caveats when the audience is, by definition, the right role.

## Personas the Coder docs serve

When deciding which audience a page targets, match the reader to one of the canonical personas the Coder docs serve.
Each persona summary captures who the reader is, what they need from the docs, and the Coder surface they typically work with.

If a page does not cleanly target one of these personas, revisit the scope.
A page without a clear persona is a page that serves no one well.

The persona names are vocabulary for writers planning a page.
They do not appear in published prose.
Inside a page, name the audience by the role the reader recognizes from their own work (`developer`, `template author`, `Coder deployment administrator`).

### Primary personas

#### Perry the Platform Engineer

Perry builds self-service platforms for development teams at a mid-to-large enterprise.
They own the templates, governance, and integrations that turn the Coder control plane Ada deploys into the default workflow developers actually use.
They need template authoring docs, RBAC and organization design, integration patterns, prebuilds, cost reporting, and policy-as-code.

*Coder surface:* template authoring (Terraform, modules, prebuilds), RBAC, organizations and groups, policy and governance, integrations (CI/CD, observability, secrets, Git), audit logs.

#### Dave the Developer

Dave is a software engineer at a company that has adopted Coder.
They were not involved in the procurement decision and are expected to use the workspace the company provisioned for them.
They need day-to-day workspace usage docs: connecting from their preferred IDE, running CLI commands inside the workspace, port forwarding, SSH, and recovering when something breaks.

*Coder surface:* workspaces, `coder` CLI, IDE integrations (VS Code, Cursor, JetBrains, Windsurf, Zed, Vim, Emacs), web terminal, dotfiles, SSH and port forwarding.

#### Elliot the End User

Elliot uses a Coder workspace day-to-day but is not a software engineer.
They may be a data scientist, product manager, customer success engineer, operations analyst, or another team member whose primary work happens inside a workspace the organization provisioned for them.
They need workspace-usage docs in plain language: connecting to the workspace, running the tools their team has standardized on, and recovering when something breaks.
They do not need template authoring or infrastructure context.

*Coder surface:* workspaces, web terminal, IDE and notebook integrations (VS Code, Jupyter, RStudio), dotfiles, port forwarding, SSH, file uploads and downloads.

> [!NOTE] Elliot is a stopgap umbrella persona for non-developer end users.
> The Coder docs team plans to revisit the persona model with product and design once the broader audience is mapped out.

#### Ada the Infrastructure Admin

Ada runs the underlying infrastructure that Coder deploys onto: Kubernetes clusters, cloud accounts, networking, storage, identity, and security policy.
They need deployment, operation, and recovery docs: install paths, upgrade and rollback, IAM and SSO, monitoring and alerting, capacity planning, and incident playbooks.
Their success metric is uptime, so they trust proven, well-documented configurations over bleeding-edge defaults.

*Coder surface:* control plane install (Helm, Docker, VM, airgapped), database, networking and DERP, IAM and SSO/OIDC/SAML, telemetry and audit logs, backup and disaster recovery.

#### Steven the Sponsor

Steven is the CTO.
They approve the Coder purchase and stay close enough to the architecture to ask sharp questions, but they no longer write code.
They need overview pages that explain what Coder is, how it fits the existing stack, what it costs, and what its security and compliance posture looks like.

*Coder surface:* architecture overviews, why-Coder framing, pricing and licensing, security and compliance summaries, release notes, success-metric dashboards.

### Secondary personas

#### Melissa the Machine Learner

Melissa is an ML engineer who lives between Jupyter notebooks, Python, ML frameworks, and large datasets.
They need docs for GPU-enabled workspaces, persistent storage for datasets and model artifacts, ML-friendly templates, and integrations with experiment tracking and model registries.
They are comfortable in the CLI but expect the dev environment to be reproducible without per-experiment setup.

*Coder surface:* GPU-enabled workspaces and templates, devcontainers, persistent storage, large-resource workspace configurations.

#### Tommy the Tester

Tommy is a QA engineer.
They need docs for reproducible test environments, CI integration patterns, and workspace configurations that let them run regression suites in isolation.
They value clear logs, traceable errors, and clean rollback paths over flashy features.

*Coder surface:* workspaces for test environments, CI integrations, reproducible build patterns, workspace lifecycle (start, stop, rebuild).

#### Caitlin the Citizen Developer

Caitlin is non-technical (customer success) but uses agentic AI tools to make small product changes without writing code.
They need docs that explain Coder Tasks and the agent-driven flows in plain language, with no assumed dev-environment knowledge and no manual setup steps.
They avoid anything that requires opening a terminal or editing a config file.

*Coder surface:* Coder Tasks, AI Gateway, prompt-driven workflows, web-based interfaces.

#### Felipe the FinOps

Felipe owns financial operations and tracks where the budget goes.
They need docs for usage and cost reporting, license counts, telemetry exports for finance dashboards, and per-team or per-template attribution.
They value precise, traceable numbers over feature descriptions.

*Coder surface:* usage reports, audit logs, license management, billing and seat counts, telemetry exports.

#### Sergio the Security Officer

Sergio is the IT security officer at an organization with strict compliance requirements.
They need docs for the security architecture, identity and access control, secrets management, audit and compliance evidence (SOC 2, FedRAMP-style controls), data residency, and the supply chain story.
They are skeptical of new tools by default and want documented, auditable behavior.

*Coder surface:* SSO and OIDC/SAML, RBAC, secrets management, audit logs, security architecture pages, compliance and trust-center content, allowlists and network policies.

#### Tara the Team Leader

Tara is an engineering manager or senior tech lead assigned the Group Admin role in Coder RBAC.
They need docs for team-scope administration: group memberships, group-owned secrets, group-scoped templates, and the audit log entries that explain who changed what.
They are not the platform owner.
They run their team inside the guardrails Perry or Ada set up.

*Coder surface:* groups, group memberships, group-owned secrets, group-scoped templates, group audit logs.

*Documentation-only.
No Vale rule.*

## Related

- [Voice and tone](./voice-and-tone.md)
- [Word choice](./word-choice.md)
- [Coder documentation content guidelines](../content-guidelines.md)
- [Style guide landing page](./README.md)
