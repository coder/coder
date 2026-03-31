# Early Access

Coder Agents is available through Early Access for the community
to evaluate while the product is under active development.
Participation comes with important expectations and limitations described
below.

## What Early Access includes

Early Access is a collaborative evaluation period between Coder and
participating customers. It includes:

- **Direct collaboration with the Coder product team** — work with Coder
  engineers and product managers to share feedback, discuss use cases, and
  influence product direction.
- **Architecture and functionality documentation** — basic documentation
  covering how Coder Agents works and how it integrates into existing
  deployments.
- **Feedback sessions** — periodic check-ins with the Coder team to discuss
  real-world usage.
- **Early exposure to new capabilities** — access to new features or
  experimental functionality before public release.

## What Early Access does not include

Early Access is not a production-ready offering. It does not include:

- **Formal support coverage** — no SLA-backed support.
- **Stability guarantees** — features and behavior may change without notice.
- **Production readiness guarantees** — functionality may not yet meet the
  reliability or scalability expectations of a GA feature.
- **Complete documentation or tooling** — operational guidance may be
  incomplete and will evolve.
- **Long-term compatibility guarantees** — APIs, configuration models, or
  workflows may change before General Availability.

## Feature scope

Functionality available during Early Access may be a subset of planned
capabilities. Some features may be incomplete, experimental, or subject to
redesign.

## Enable Coder Agents

Coder Agents is experimental and must not be deployed to production
environments. It is gated behind the `agents` experiment flag. To enable it,
pass the flag when starting the Coder server using an environment variable
or CLI flag:

```sh
CODER_EXPERIMENTS="agents" coder server
# or
coder server --experiments=agents
```

If you are already using other experiments, add `agents` to the
comma-separated list:

```sh
CODER_EXPERIMENTS="agents,oauth2,mcp-server-http" coder server
```

Once the server restarts with the experiment enabled:

1. Navigate to the **Agents** page in the Coder dashboard.
1. Open **Admin** settings and configure at least one LLM provider and model.
   See [Models](./models.md) for detailed setup instructions.
1. Grant the **Coder Agents User** role to users who need to create chats.
   Go to **Admin** > **Users**, click the roles icon next to each user,
   and enable **Coder Agents User**.
1. Developers can then start a new chat from the Agents page.

## Licensing and availability

Features provided during Early Access may become paid licensed
features at General Availability.
Participants will receive reasonable advance notice before:

- Coder Agents reaches General Availability
- Early Access functionality transitions to a paid offering

## Providing feedback

Participants are encouraged to share workflow feedback, feature requests,
performance observations, and operational challenges. Feedback channels are
coordinated directly with the Coder product team.
