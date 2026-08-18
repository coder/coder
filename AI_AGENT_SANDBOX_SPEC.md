# AI Agent Workspace Isolation

Status: design specification. Nothing described here should be assumed to
exist. Every section is written as work to be done, in normative voice, so
that an implementer can build it from this document alone. Where the
document refers to behavior Coder already has independently of this
feature, it says so explicitly.

Companion document: `AI_AGENT_IDENTITY_SPEC.md`. That document specifies
who an agent is, how identities are minted, and how authorization narrows.
This document specifies where an agent runs, what credentials reach that
environment, and how its network egress is controlled. It depends on the
identity spec and does not restate it.

## Problem

An AI agent running in a workspace inherits everything the workspace has,
because workspaces were designed for the humans who own them:

- The workspace agent's manifest carries the owner's user secrets, and the
  agent API serves the owner's external-auth tokens and Git SSH key to
  anything holding the agent token.
- Templates conventionally place a full-scope owner session token in the
  workspace environment, so a process inside can act as the owner against
  the whole deployment.
- Nothing constrains where a process in a workspace may connect. An agent
  that pulls a malicious dependency, or is steered by a prompt injection,
  can exfiltrate whatever it can read to anywhere it can reach.

The last point is what makes the first two urgent. A human developer with
credentials in their workspace is a human exercising their own access. An
agent with the same credentials is an unattended process whose behavior is
determined partly by text it reads at runtime.

## Solution

Two independent mechanisms, deliberately separable so that each can be
adopted alone:

1. **Credential starvation.** An environment an AI agent controls receives
   none of the sponsor's ambient credentials. It gets a scoped,
   workspace-pinned AI session token instead. This is enforced
   server-side, per workspace agent, at every credential source.
2. **Egress control.** Traffic from a confined environment is routed
   through a policy proxy that allows a default-deny host allowlist,
   records every allowed and denied flow, and is configured from
   template settings that no one inside the workspace can influence.

Both attach to the same marker: a workspace agent bound to an AI identity.
Binding is the discriminator throughout this document, and it must be
server-authoritative. Nothing inside a workspace may cause an agent to
become unbound.

## Shapes

Two arrangements were designed. **Only the second is in scope**, and the
distinction matters because they need different boundaries for different
reasons.

| Shape                                             | Outer boundary                                                           | Inner boundary                                 | Status       |
|---------------------------------------------------|--------------------------------------------------------------------------|------------------------------------------------|--------------|
| AI-designated workspace                           | the Terraform-provisioned container or VM is itself the AI's environment | network namespace around the agent             | deferred     |
| Sandboxed AI agent in an ordinary human workspace | the human's workspace                                                    | container or microVM built by a startup script | **in scope** |

Three reasons the sandbox shape is the one to build first:

1. **It is the shape that needs a second boundary.** In an AI-designated
   workspace the AI is the sole occupant, so the workspace container or VM
   already isolates it from everything except the network; adding a
   namespace inside that protects nothing new. In a human workspace the AI
   is a guest, so process and filesystem isolation are required, not just
   network isolation.
2. **It keeps the supervisor and the confined party distinguishable.**
   Designation binds every workspace agent of every build, so in an
   AI-designated workspace the supervising agent would be bound too, and
   binding could no longer separate "confining, receives policy" from
   "confined, receives nothing". Restoring that distinction would need a
   second discriminator. The sandbox shape gets it for free: the host
   agent is unbound, the sandbox's agent is bound.
3. **It has lower host requirements.** A namespace-based inner boundary
   needs `NET_ADMIN` and iproute2 in the workspace image. The sandbox
   shape's requirements land on the workspace image chosen by the
   administrator rather than on the platform.

The deferred combination is "an AI-designated workspace that also runs a
sandbox", which is deferred for reason 2, not because it is uninteresting.

Honest consequence of the reduction: the retained shape is **attested**
rather than structurally enforced. The startup script builds the boundary,
and the platform records what the administrator claims about it without
verifying the claim.

## Declaration model

Everything about a sandbox is declared in the template and created by the
build. There is no runtime sandbox-creation API.

```hcl
# The host agent. Ordinary, unbound, full owner credentials. This is the
# egress supervisor, so it is the agent that receives egress policy.
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

# The AI's agent. ai_bound is the entire security-relevant declaration:
# the server mints a sandbox identity for it, withholds every ambient
# owner credential, and never delivers egress policy to it.
resource "coder_agent" "ai" {
  os                 = "linux"
  arch               = "amd64"
  ai_bound           = true
  egress_enforcement = "forced" # forced | advisory | none
}

# The sandbox is brought up by an ordinary startup script. The platform
# supplies the values it needs at exec time.
resource "coder_script" "sandbox" {
  agent_id     = coder_agent.main.id
  display_name = "AI sandbox"
  run_on_start = true
  run_on_stop  = true
  script       = file("${path.module}/scripts/sandbox.sh")
}

# Apps and further scripts attach to the bound agent directly.
resource "coder_app" "ai_terminal" {
  agent_id = coder_agent.ai.id
  slug     = "ai-terminal"
}
```

Requirements on the declaration surface:

- **`ai_bound` is opt-in only and monotonic.** It can withhold credentials
  but never grant them. `false` must be equivalent to omitted, and must
  never unbind an agent in a workspace the server has already designated;
  otherwise a template edit becomes an escalation path.
- **`egress_enforcement` has no default.** Empty means "not declared",
  which the server must distinguish from a declared `none`. It is an
  administrator attestation, recorded and surfaced but never verified.
  Administrators may set a template-level floor requiring `forced`.
- **Egress rules appear nowhere in Terraform.** They live in template
  settings, are versioned, and are editable without a template push. A
  template must not be able to author its own egress exceptions.
- **The startup script is an ordinary `coder_script`.** Its content is
  stored and versioned with the template, its output is surfaced as script
  logs, and it is not staged into the image out of band.

### Why a startup script rather than a first-class sandbox resource

The mechanism is a script either way, so modeling it as a distinct
resource type would add a platform concept that owns nothing the script
runner does not already own: content storage, log surfacing, ordering, and
`run_on_stop`. Two arguments for a first-class resource were considered
and are recorded as future work rather than accepted:
provisioner-attested properties (`egress_enforcement` declared on the
agent achieves the same thing, since the agent is also provisioner
output), and a `parent_id` relationship between the host agent and the
sandbox's agent (which would additionally buy nested presentation in the
UI and the existing sub-agent deletion checks).

## Credential starvation

Binding must activate fail-closed credential handling at every source of
ambient owner credentials. There are four, and they are independent: a
control that covers only the manifest is insufficient, because the others
are separately reachable API routes.

| Source                | Requirement                                                                                                                                                                                                                                            |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Manifest user secrets | Omit the owner's secrets for a bound agent. Note this read is typically performed with elevated system access precisely because it bypasses normal scope checks, so it needs an explicit binding check.                                                |
| External auth tokens  | Deny a bound caller.                                                                                                                                                                                                                                   |
| Git SSH key           | Deny a bound caller.                                                                                                                                                                                                                                   |
| Owner session token   | A designated workspace must never receive the ambient full-owner token, and a bound agent must never receive a copy from its unbound host. This is a separate control from the three above, governed by designation rather than by per-request checks. |

Requirements:

1. **The predicate is one function.** All four sites must consult a single
   shared predicate over the agent row so that they cannot drift apart.
2. **Set the binding at insert.** Credential starvation is enforced per
   agent row, so a binding applied by a later `UPDATE` must be remembered
   by every path that creates an agent row. Make the binding a parameter
   of the agent insert, and have child-creation paths inherit the parent's
   binding. If the codebase enforces exhaustive struct literals, this
   turns "did every creation path remember to bind?" into a compile-time
   question.
3. **Missing information denies.** An unresolvable identity, absent
   binding metadata, or an unknown credential mode must deny rather than
   fall back to human-agent behavior.
4. **Bound agents authorize as the agent.** The workspace-agent
   authentication path must build the delegated subject described in the
   identity spec, and must additionally force the no-user-data scope so
   that a bound agent cannot reach user-data routes even if a template
   declared otherwise.

### Credential modes

A single boolean is enough for the PoC, but the field should be an enum
from the start because the middle case is already known to be wanted:

| Mode       | Meaning                                                                                                 |
|------------|---------------------------------------------------------------------------------------------------------|
| `none`     | No third-party credentials. The default, and the only mode required for the PoC.                        |
| `injected` | Specific credentials are placed in the environment, requiring explicit sponsor consent.                 |
| `brokered` | The agent reaches third parties through a broker that holds the credential, so the agent never sees it. |

The server must clamp the declared mode: a template may narrow but never
widen what the deployment permits.

## Egress control

### Topology

```text
[ workspace container ]
  host agent  ── runs policy proxy, holds policy, reports events
       │
       │  startup script builds the boundary and starts the sandbox
       ▼
[ sandbox: container or microVM ]
  bound agent + AI process ── no ambient credentials, no policy,
                              egress routed to the host proxy
```

The host agent is the **egress supervisor**: it holds policy, runs the
proxy, and reports network events. The sandbox is the **confined party**.
The two roles must be distinguishable from data the server owns, which is
what binding provides.

### Ordering requirement

The proxy must be listening, and the boundary must be in place, before the
confined process can make its first connection. Concretely the host agent
must, in this order: obtain policy, start the proxy, then run the startup
script that builds the boundary and launches the sandbox. A window in
which the sandbox is running but unconfined is a correctness failure, not
a performance detail.

### Policy storage and delivery

Egress policy lives in **template settings**, not in Terraform. It is a
versioned object that administrators edit through the API without a
template push or a workspace rebuild: default-deny, with implicit allows
for the control plane and AI gateway, a template-level allowlist, and
bounded per-workspace overrides. Every write is audited. The only writers
are server-side actors, administrators today and human-in-the-loop
approval flows later. Because the writer is never the workspace, runtime
**widening** is safe, which is what makes future approval flows work
without restarts.

Delivery is one mechanism at two moments:

1. **Bootstrap.** The host agent receives the current policy with its
   manifest. The manifest is the natural channel because the host agent is
   the supervisor and the manifest is already gated on binding: an unbound
   agent receives owner credentials and policy, a bound agent receives
   neither. Reusing that predicate makes "the confined party never
   consumes policy" a property enforced by a tested gate rather than by
   convention.
2. **Runtime updates.** Revisions are pushed to the supervisor over its
   existing connection and applied atomically to the running proxy. The
   confined process is untouched: no restart, no rebuild.

**The confined party never consumes policy.** State the rule in terms of
the confinement relationship rather than in terms of a channel, because
the supervisor role can be filled differently in different shapes. Any
policy-read endpoint must therefore deny a bound caller, independently of
which channel a shape uses for bootstrap.

Fetch failure must produce deny-all plus a degraded report. It must never
produce an unconfined sandbox.

### Host-side policy export contract

Some sandbox runtimes cannot route the confined guest through the Coder
confine proxy. A host-side runtime such as `coder/sandbox` may enforce the
same template policy in its own proxy instead. This mode does not deliver
policy to the confined agent or guest. The unbound host agent exports policy
to a host-only file consumed by the sandbox daemon.

The contract is opt-in through two agent process environment variables:

| Variable                                | Purpose                                                                                        |
|-----------------------------------------|------------------------------------------------------------------------------------------------|
| `CODER_AI_SANDBOX_POLICY_FILE`          | Enables export and names the host-side file that receives a `runtime.network` YAML document.   |
| `CODER_AI_SANDBOX_POLICY_RELOAD_SCRIPT` | Optional command run after each successful file replacement so an external runtime can reload. |

When `CODER_AI_SANDBOX_POLICY_FILE` is set, the sandbox controller must:

1. Translate and atomically replace the file after the initial policy fetch.
   A fetch failure still writes the deny-all bootstrap policy and reports
   degraded.
2. Complete the initial replacement before invoking
   `CODER_AI_SANDBOX_CREATE_SCRIPT`, so the sandbox cannot boot without its
   first policy document.
3. Repeat the translation and replacement for every complete policy revision
   received from the SSE watch.
4. Use a temporary file in the destination directory followed by rename.
   Equivalent policies must produce byte-identical output.
5. After each successful replacement, run
   `CODER_AI_SANDBOX_POLICY_RELOAD_SCRIPT` when declared. The hook has a
   30-second timeout and receives `CODER_SANDBOX_ID` and
   `CODER_AI_SANDBOX_POLICY_FILE` in its environment. Hook failure is logged
   and reported as degraded, but is not fatal and must not disable later
   updates.

The exported file is the mapping used at `runtime.network`, not a complete
sandbox descriptor:

```yaml
reload: watch
default: deny
mode: enforce
rules:
  - host: api.example.com
    ports: [443]
    action: allow
    tls: passthrough
```

The translation is deterministic and has these semantics:

| Coder policy input                    | Exported `runtime.network` rule                                                                                               |
|---------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| Any policy                            | `default: deny`, `mode: enforce`, and `reload: watch`                                                                         |
| Host with an empty port list          | Explicit `ports: [80, 443]`; an omitted list in `coder/sandbox` would mean any port                                           |
| Exact hostname                        | Allow rule for that hostname and the declared ports                                                                           |
| Leading `*.` wildcard                 | Pattern is passed through unchanged. `coder/sandbox` matches arbitrary subdomain depth, which is wider than Coder's one label |
| IPv4 or IPv6 literal                  | Exact `/32` or `/128` CIDR allow rule                                                                                         |
| Every translated allow                | `tls: passthrough`, preserving end-to-end TLS without the sandbox runtime's interception CA                                   |
| Coder access URL                      | Allow the hostname on its effective port                                                                                      |
| Private or loopback access URL result | Also allow each resolved address as an exact `/32` or `/128` CIDR on the access URL port                                      |

Access URL resolution must have a context deadline. Resolution failure logs a
warning but does not prevent the hostname rule or file replacement. The exact
CIDRs are required because the external proxy performs a second CIDR decision
when a permitted hostname resolves to a private or loopback address.

The Coder proxy, SSE watcher, and event batcher continue running in this mode.
They remain the control path for policy revisions, even when the guest's
network cannot reach the Coder proxy.

For `coder/sandbox`, `reload: watch` watches the descriptor path registered by
`coder-sandbox up NAME -f descriptor.yaml`, not a separate policy file. The
daemon polls that descriptor every 500 milliseconds, projects only its
`runtime` section, and atomically reloads the shared network and MCP policy.
The descriptor schema has no external-file include for `runtime.network`, so
the reload hook must re-render the descriptor with the exported mapping.
Changes under `sandbox` do not alter a running VM.

### Policy content

Two layers:

- **L4 for the control plane.** The Coder server and DERP must be
  reachable, or the bound agent cannot connect at all. Confined agents
  should default to relay-only tailnet behavior, because direct
  peer-to-peer traffic conflicts with default-deny UDP. Ingress for SSH
  and apps rides the agent's existing connection and is unaffected.
- **L7 for everything else.** Host-level allow, deny, and audit decisions
  from CONNECT targets and TLS SNI, without decryption. Matching should be
  exact host or a single leading-label wildcard, with empty ports implying
  the standard HTTP and HTTPS ports.

The proxy must resolve each destination once and validate it before
dialing, rejecting loopback, link-local, private, and cloud metadata
ranges, so that an allowed hostname cannot be used to reach the
supervisor's own network position.

### Attestation, not verification

`egress_enforcement` is what the administrator claims the startup script
built:

| Value      | Claim                                                                     |
|------------|---------------------------------------------------------------------------|
| `forced`   | Every egress path from the sandbox is routed through the proxy.           |
| `advisory` | Proxy environment variables are set; a process that ignores them escapes. |
| `none`     | No claim.                                                                 |

The platform records and surfaces the claim through API and UI and must
not present it as verified. Detecting a mismatch, for example an
attested-`forced` sandbox whose proxy sees no traffic while the AI is
active, is auditing work and belongs to the auditing vertical.

### Proxy access control

This is a requirement the shape reduction introduces, and it is easy to
miss. In a namespace-based boundary, the namespace itself is the proxy's
access control: only the confined child can route to the listener. A
container-based sandbox generally cannot reach the host agent's loopback
address, so the proxy must bind a reachable interface, which removes that
boundary.

The proxy must therefore authenticate its clients. Issue a per-sandbox
bearer token, require it on CONNECT and on forwarded requests, and reject
unauthenticated clients. Without it, any process that can reach the
listener can use the workspace's egress allowlist, and, more seriously,
its traffic is recorded against the sandbox's session, which corrupts
attribution for the auditing vertical.

## What the platform supplies to the startup script

The script cannot be self-contained: it needs values that do not exist
until the agent is running, and it must not contain credentials, because a
script body is stored server-side and readable through the API. The agent
must therefore inject these at exec time rather than the template
interpolating them:

| Variable                 | Purpose                                                                                        |
|--------------------------|------------------------------------------------------------------------------------------------|
| `CODER_AI_AGENT_URL`     | Control plane URL for the bound agent inside the sandbox.                                      |
| `CODER_AI_AGENT_TOKEN`   | The bound agent's token, so the sandbox's agent can connect.                                   |
| `CODER_AI_SESSION_TOKEN` | The scoped AI session token for in-sandbox CLI use. Supplied to the start script only.         |
| `CODER_EGRESS_PROXY`     | Address of the supervisor's proxy. Chosen when the proxy binds, so it cannot be known earlier. |
| `CODER_SANDBOX_ID`       | Identifier for correlating audit records.                                                      |

Two requirements follow:

1. **Exec-time resolution.** The values must be computed when the script
   runs. A template that interpolated a token into a script body would
   place a live credential in the script record and in Terraform state.
2. **Asymmetric teardown environment.** A stop script must not receive the
   session token. Teardown has no need for a live credential, and the
   difference is only expressible if the environment is decided per
   execution.

## Invariants

Drive tests from these. The identity spec's invariants also apply and are
not repeated.

1. **Server-authoritative binding.** Only server-side data can set or
   interpret an agent's binding and credential mode. No in-workspace input
   can unbind an agent or change its credential disposition.
2. **No ambient human credentials for bound agents.** A bound agent is
   denied owner user secrets, external-auth tokens, Git SSH keys, and the
   ambient owner session token. Missing policy or resolution errors deny.
3. **Binding covers every creation path.** Every workspace agent row,
   however created, carries the binding its parent or its workspace
   designation implies. Test each creation path, including runtime
   sub-agent creation, which is authenticated by a token that is present
   in the agent environment and therefore reachable by a process the AI
   controls.
4. **Credential separation.** The workspace agent token governs the agent
   daemon; the scoped AI session token governs in-workspace CLI actions.
   Neither may silently substitute for the other.
5. **Default-deny egress.** A fresh sandbox can reach the control plane
   and nothing else. Every allowed or denied flow produces an attributed
   audit record when the shape attests `forced`.
6. **The confined party never consumes policy.** The sandbox never
   fetches, holds, or sees egress policy, on any channel.
7. **Ordering.** The proxy is listening and the boundary is in place
   before the confined process can connect.
8. **Fail closed on policy.** A policy fetch failure yields deny-all and a
   degraded report, never an unconfined sandbox.
9. **Attestation honesty.** A declared enforcement level is recorded and
   surfaced as a claim. The platform does not verify it and must not
   present it as verified.
10. **Durable attribution.** Bound-agent requests and background events
    record actor equals agent and on-behalf-of equals sponsor. Egress
    records survive identity cleanup.

## Implementation order

1. **Prerequisite safety fixes.** Fail-closed handling at all four
   credential sources, keyed on a single shared predicate. Land the
   minimum binding schema and query support these need in the same
   increment, including making the binding a parameter of the agent
   insert.
2. **Declaration plumbing.** `ai_bound` and `egress_enforcement` on the
   agent resource in the Terraform provider; the corresponding fields in
   the provisioner protocol; parsing in the provisioner; and identity
   resolution plus binding at build completion, where the workspace, its
   owner, and its organization are all available.
3. **Bound-agent authentication.** Resolve the bound identity in
   workspace-agent authentication, build the delegated subject, force the
   no-user-data scope, and plumb agent and sponsor attribution through
   request and background audit events.
4. **Policy object and delivery.** The versioned template-settings policy,
   admin API with audit, manifest bootstrap gated on binding, and the
   runtime update stream. Deny policy reads to bound callers.
5. **Proxy.** Policy engine and matcher, CONNECT and absolute-form
   handling, destination validation, per-sandbox client authentication,
   and the retained event stream. Start it in the host agent before
   scripts run, and expose its address to scripts as exec-time
   environment.
6. **Script contract.** Exec-time environment injection for start and stop
   scripts, with the session token supplied to start only. Reference
   sandbox scripts for at least one container backend and one microVM
   backend, with their host requirements documented.
7. **Credential modes.** The enum, mode-specific exceptions, and sponsor
   consent for `injected`.
8. **Surfacing.** Binding and attestation in the workspace agent API,
   workspace page, and CLI; egress activity views.

Steps 1 to 3 are independently mergeable and deliver credential
starvation with no egress control. Steps 4 to 6 deliver egress control.
Step 5 depends on step 4's policy object but not on step 6.

## Non-goals

- Verifying attestations. The platform records claims; detecting
  mismatches is auditing work.
- Human-in-the-loop egress approvals. The policy object already permits
  runtime widening by server-side writers, so approvals need no new
  delivery mechanism; the approval API and UI land later.
- Deployment-level transparent TLS interception.
- L7 method or path rules, which need a sound guest certificate authority
  boundary.
- More than one sandbox per workspace in practice, though nothing in the
  design forbids it.
- Snapshots and checkpointing; Windows and macOS sandbox backends.
- Adding a sandbox to a running workspace without a rebuild. This is the
  cost of the declaration model.

## Known gaps and decisions still open

- **MicroVM event retention.** In host-side policy export mode, the guest
  uses the external runtime's proxy rather than the Coder confine proxy.
  The external runtime may record requests locally, but those flows are not
  yet retained as Coder AI sandbox network events.
- **MicroVM IPv6 enforcement.** The current `coder/sandbox` VM firewall
  parses and restricts IPv4 guest egress, but forwards IPv6 and other
  non-IPv4 EtherTypes without equivalent policy enforcement. A template
  using this backend must not attest `forced` until that gap is closed.
- **Wildcard widening.** Coder egress policy gives `*.example.com`
  single-label semantics. `coder/sandbox` treats the same syntax as an
  arbitrary-depth suffix. Policy export preserves the pattern, so this
  backend knowingly widens wildcard matches.
- **External policy inclusion.** The `coder/sandbox` descriptor cannot
  include the exported `runtime.network` mapping from another file. The
  reload hook must re-render a complete descriptor, adding integration
  complexity and another file replacement step.
- **Proxy client authentication** is specified above but is the single
  largest gap between this document and a sound implementation of the
  container backend.
- **Agent token reachability.** The host agent's token is deliberately
  placed in the agent environment and inherited by spawned processes, so a
  process the AI controls can call agent APIs as the host agent. Scrubbing
  it from AI-visible environments is worth investigating, but it may break
  `coder` subcommands that legitimately rely on it.
- **Create-script failure semantics.** A failed sandbox build currently
  has two defensible readings: leave the workspace running without the
  sandbox and report degraded, or fail the build. The analogous
  structural-confinement path treats setup failure as fatal. Decide
  deliberately.
- **Cross-sandbox isolation** within one workspace, if more than one
  sandbox is ever used, is unspecified.
- **The deferred namespace shape** remains desirable for AI-designated
  workspaces and needs the supervisor-versus-confined discriminator
  described in "Shapes" before it can be built.
