# Virtual desktop

> [!NOTE]
> This feature is experimental. Pin a release before broad rollout and review
> the release notes before upgrading.

## Enable the experiment

```shell
coder server --experiments=chat-virtual-desktop
```

Or set the environment variable:

```shell
CODER_EXPERIMENTS=chat-virtual-desktop
```

## What it does

Lets agents drive a graphical desktop inside the workspace through
`spawn_agent` with `type=computer_use` (screenshots, mouse, keyboard).

## Configuration

Once the experiment is enabled, configure virtual desktop under **Agents** >
**Settings** > **Manage Agents** > **Agents**.

To enable, toggle **Virtual Desktop** on, then choose a **Computer use
provider** (Anthropic or OpenAI). It also requires:

- The [portabledesktop](https://registry.coder.com/modules/coder/portabledesktop)
  module installed in the workspace template.
- An API key for the selected provider configured under the **Providers**
  tab.

The Anthropic and OpenAI computer-use models are fixed by Coder per provider
and are not selectable from this UI. Anthropic is the default when no
provider is set.

The same configuration is available at:

- `GET /api/experimental/chats/config/desktop-enabled`
- `PUT /api/experimental/chats/config/desktop-enabled`
- `GET /api/experimental/chats/config/computer-use-provider`
- `PUT /api/experimental/chats/config/computer-use-provider`
