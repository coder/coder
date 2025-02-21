# Underlying networking stack

The underlying networking stack includes [Wireguard](https://www.wireguard.com/) (implemented through [Tailscale](https://tailscale.com)), DERP, and STUN.
Additionally, a Coder agent runs within workspaces to establish connections to the Coder server.

## Wireguard through Tailscale

Establishes Wireguard tunnels between the client, server, and provisioners.

This includes the following protocols:

- DERP:
  - Built-in "relays” that help route connections from the client to the workspace through the central Coder server or workspace proxies.

     Each server and workspace proxy has a DERP server included. We do not really recommend self-hosting DERP outside of what is bundled in Coder, but it is possible. We have docs on how to use Tailscale’s public DERP servers, but this is off by default.
- STUN:
  - Helps establish direct peer-2-peer connections.

     No user traffic is routed through it.
     By default, the Coder server will attempt to use Google’s STUN servers.

This implementation of Wireguard ensures portable networking that “just works” without lots of configuration.
Administrators only need to open one HTTPS port on the server instead of needing to set up different relays for HTTPS, SSH, UDP, Generic TCP, and others.
It works for global deployments or single deployments with a mesh network architecture, and users can establish direct peer-to-peer connections when enabled.

### Wireguard / Tailscale FAQ

- Does Coder reach out to Tailscale’s servers?
  - No

- Does this networking work offline?
  - Yes

- Does this networking work with <insert cloud / air gapped deploymnets>?
  - Yes

- Does this networking work with <insert VPN>?
  - Yes

- Does this networking work with <browser>?
  - Yes

- Can direct connections be disabled?
  - Yes

- Can SSH be disabled or browser only be enforced?
  - Yes

- Can I use something besides Tailscale for connections?
  - Technically, yes. But this is not well documented.
  
    The agent uses Tailscale to stream logs, but users can technically connect to your workspaces via port 22 and SSH keys and not use Coder’s authentication. We are exploring ways to allow a “hybrid” approach where Tailscale is used for internal log streaming but users must connect via generic SSH, Teleport, or similar.

- Are direct connections faster than relayed?
  - Yes

## Coder agent

Coder agent runs within workspaces to establish a connection to the Coder server so that users can connect to their workspace and access things within the workspace such as the terminal and ports running on the workspace.
This connection between the user and the workspace always requires an authenticated Coder session.

As long as the agent can reach port 443 on the Coder server, a connection can be established between the agent and the coderd process.
The agent typically uses a token to authenticate with coderd.
