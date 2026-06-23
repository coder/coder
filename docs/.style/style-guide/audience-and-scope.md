# Audience and scope

Every page in the Coder documentation targets one audience working toward one outcome.
The audience determines vocabulary, depth, and the prior knowledge the page assumes.
The outcome determines what the page covers and where it stops.

Pages that try to serve two audiences,
or chain multiple unrelated outcomes,
serve none of their readers well.
A reader who is one persona away from the page's target
has to skip past content that does not apply to them,
guess which sentences are for them,
and trust the writer not to have buried a step they need
inside a section labeled for someone else.

The single canonical Coder example is
**install Coder**.
A workspace user wants to connect their local editor to a Coder workspace and start coding.
A platform engineer wants to deploy the Coder control plane to their company's Kubernetes cluster.
Both groups search for "install Coder."
A page that tries to cover both
forces the workspace user to read past Helm chart values,
and forces the platform engineer to read past Visual Studio Code download links.
Two pages,
one per audience and one per outcome,
serve both groups better than one page that combines them.

## Pick one audience per page

Choose the audience before you choose the words.
The audience determines:

- The product vocabulary the reader already knows
  (for example, whether `workspace` needs a definition).
- The infrastructure context the reader brings
  (for example, whether Kubernetes is assumed).
- The level of depth the reader expects
  (overview, how-to, reference, or in-depth tutorial).

If a topic genuinely needs to serve two audiences,
write two pages and cross-link them.
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

The outcome is the specific task,
or the small set of related tasks,
the page helps the reader accomplish.
A how-to page covers one task.
A tutorial covers one chained workflow.
A reference page covers one stable surface
(one CLI command, one API endpoint, one schema).
An overview page introduces one concept.

If the page has more than one outcome,
split it.
"Configure SSO with Okta" is one outcome.
"Configure SSO" is not.
"Deploy Coder on AWS" is one outcome.
"Deploy Coder" is not.

A page that helps the reader accomplish two unrelated outcomes
hides each outcome from the readers who need the other.

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

## Declare audience and scope up front

The first paragraph of the page names the audience and the outcome.
The reader should know within the first two or three sentences
whether the page is for them
and whether it covers their task.

Conventions:

- The H1 names the outcome.
- The first paragraph names the audience and confirms the outcome.
- The first paragraph also links to sibling pages for adjacent audiences or outcomes when those exist.

**Do**:

```markdown
# Deploy Coder on Kubernetes with the Helm chart

This guide walks a platform engineer through deploying the Coder control plane to a Kubernetes cluster
using the official Helm chart.
It assumes you have `kubectl` and `helm` configured against the target cluster.

For a managed install on a single VM, refer to [Install Coder on a virtual machine](./vm.md).
For Coder Cloud, refer to [Get started with Coder Cloud](./cloud.md).
```

**Don't**:

```markdown
# Kubernetes

Coder runs on Kubernetes.
This page covers many topics related to running Coder on Kubernetes.
```

## Personas the Coder docs serve

When deciding which audience a page targets,
match the reader to one of the canonical personas the Coder docs serve.
Each persona summary captures who the reader is,
what they need from the docs,
and the Coder surface they typically work with.

If a page does not cleanly target one of these personas,
revisit the scope.
A page without a clear persona is a page that serves no one well.

### Primary personas

#### Dave the Developer

Dave is a software engineer at a company that has adopted Coder.
He was not involved in the procurement decision and is expected to use the workspace the company provisioned for him.
He needs day-to-day workspace usage docs: connecting from his preferred IDE, running CLI commands inside the workspace, port forwarding, SSH, and recovering when something breaks.

*Coder surface:* workspaces, `coder` CLI, IDE integrations (VS Code, Cursor, JetBrains, Windsurf, Zed, Vim, Emacs), web terminal, dotfiles, SSH and port forwarding.

#### Ada the Infrastructure Admin

Ada runs the underlying infrastructure that Coder deploys onto: Kubernetes clusters, cloud accounts, networking, storage, identity, and security policy.
She needs deployment, operation, and recovery docs: install paths, upgrade and rollback, IAM and SSO, monitoring and alerting, capacity planning, and incident playbooks.
Her success metric is uptime, so she trusts proven, well-documented configurations over bleeding-edge defaults.

*Coder surface:* control plane install (Helm, Docker, VM, airgapped), database, networking and DERP, IAM and SSO/OIDC/SAML, telemetry and audit logs, backup and disaster recovery.

#### Perry the Platform Engineer

Perry builds self-service platforms for development teams at a mid-to-large enterprise.
He owns the templates, governance, and integrations that turn the Coder control plane Ada deploys into the default workflow developers actually use.
He needs template authoring docs, RBAC and organization design, integration patterns, prebuilds, cost reporting, and policy-as-code.

*Coder surface:* template authoring (Terraform, modules, prebuilds), RBAC, organizations and groups, policy and governance, integrations (CI/CD, observability, secrets, Git), audit logs.

#### Steven the Sponsor

Steven is the CTO.
He approves the Coder purchase and stays close enough to the architecture to ask sharp questions, but he no longer writes code.
He needs overview pages that explain what Coder is, how it fits the existing stack, what it costs, and what its security and compliance posture looks like.

*Coder surface:* architecture overviews, why-Coder framing, pricing and licensing, security and compliance summaries, release notes, success-metric dashboards.

### Secondary personas

#### Melissa the Machine Learner

Melissa is an ML engineer who lives between Jupyter notebooks, Python, ML frameworks, and large datasets.
She needs docs for GPU-enabled workspaces, persistent storage for datasets and model artifacts, ML-friendly templates, and integrations with experiment tracking and model registries.
She is comfortable in the CLI but expects the dev environment to be reproducible without per-experiment setup.

*Coder surface:* GPU-enabled workspaces and templates, devcontainers, persistent storage, large-resource workspace configurations.

#### Tommy the Tester

Tommy is a QA engineer.
He needs docs for reproducible test environments, CI integration patterns, and workspace configurations that let him run regression suites in isolation.
He values clear logs, traceable errors, and clean rollback paths over flashy features.

*Coder surface:* workspaces for test environments, CI integrations, reproducible build patterns, workspace lifecycle (start, stop, rebuild).

#### Caitlin the Citizen Developer

Caitlin is non-technical (customer success) but uses agentic AI tools to make small product changes without writing code.
She needs docs that explain Coder Tasks and the agent-driven flows in plain language, with no assumed dev-environment knowledge and no manual setup steps.
She avoids anything that requires opening a terminal or editing a config file.

*Coder surface:* Coder Tasks, AI Bridge, prompt-driven workflows, web-based interfaces.

#### Felipe the FinOps

Felipe owns financial operations and tracks where the budget goes.
He needs docs for usage and cost reporting, license counts, telemetry exports for finance dashboards, and per-team or per-template attribution.
He values precise, traceable numbers over feature descriptions.

*Coder surface:* usage reports, audit logs, license management, billing and seat counts, telemetry exports.

#### Sergio the Security Officer

Sergio is the IT security officer at an organization with strict compliance requirements.
He needs docs for the security architecture, identity and access control, secrets management, audit and compliance evidence (SOC 2, FedRAMP-style controls), data residency, and the supply chain story.
He is skeptical of new tools by default and wants documented, auditable behavior.

*Coder surface:* SSO and OIDC/SAML, RBAC, secrets management, audit logs, security architecture pages, compliance and trust-center content, allowlists and network policies.

#### Tara the Team Leader

Tara is an engineering manager or senior tech lead assigned the Group Admin role in Coder RBAC.
She needs docs for team-scope administration: group memberships, group-owned secrets, group-scoped templates, and the audit log entries that explain who changed what.
She is not the platform owner.
She runs her team inside the guardrails Perry or Ada set up.

*Coder surface:* groups, group memberships, group-owned secrets, group-scoped templates, group audit logs.

*Documentation-only. No Vale rule.*

## Related

- [Voice and tone](./voice-and-tone.md)
- [Word choice](./word-choice.md)
- [Coder documentation content guidelines](../content-guidelines.md)
- [Style guide landing page](./README.md)
