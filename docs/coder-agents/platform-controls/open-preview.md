---
title: Workspace previews
---

> [!NOTE]
> Workspace previews are experimental.
> The `open_preview` tool and the way previews behave can change in any release.

Workspace previews let the agent in a chat show a web page from the workspace in a tab in the chat's right panel.
This page is for Coder deployment administrators who turn on workspace previews.

## Enable workspace previews

Workspace previews have three requirements on the control plane:

- The control plane has a [wildcard access URL](../../admin/networking/wildcard-access-url.md).
  Without one, `open_preview` returns `previews need a wildcard app hostname on the primary deployment; tell the user an administrator can set one`.
  The wildcard access URL must be on the same site as the access URL, which means under the same registrable domain, such as `*.coder.example.com` for `coder.example.com`.
  Otherwise, preview tabs show `Previews need a wildcard access URL on the same site as the dashboard.` instead of the preview.
- The access URL uses HTTPS.
  Over plain HTTP, the preview tab shows an error page that says `Previews need an HTTPS access URL.` instead of the preview.
  For local development, plain HTTP works when the wildcard access URL is under `localhost`, such as `*.coder.localhost` with the access URL `http://coder.localhost:3000`.
- The control plane runs with [`--host-prefix-cookie`](../../reference/cli/server/index.md#--host-prefix-cookie) (`CODER_HOST_PREFIX_COOKIE=true`), which stops scripts in a preview from replacing the user's Coder session cookie.
  Without the flag, `open_preview` returns `previews require --host-prefix-cookie; tell the user an administrator can enable it`, and preview tabs show `Previews require --host-prefix-cookie. An administrator can enable it.` instead of the preview.
  If the `open-preview` experiment is on, Coder also logs `workspace previews are unavailable` at startup.

File previews also need a workspace agent from the same release as the control plane.
A workspace gets that agent when it restarts after you upgrade Coder.
Until the workspace restarts, `open_preview` returns `the workspace agent is outdated; restart the workspace to preview files` for a file preview.

To give the agent the tool, enable the `open-preview` experiment on the control plane:

```sh
coder server --experiments=open-preview
```

Or set the environment variable:

```sh
CODER_EXPERIMENTS=open-preview
```

If you already enable other experiments, add `open-preview` to the list, such as `--experiments=chat-virtual-desktop,open-preview`.
The `*` value doesn't enable `open-preview`.
Refer to [Feature stages](../../reference/feature-stages.md) for how experiments work.
Restart the control plane after you change the experiment list.
While the experiment is off, the agent doesn't get the tool and Coder doesn't serve previews.

### Workspace proxies

A preview loads through the [workspace proxy](../../admin/networking/workspace-proxies.md) that the user selects in the dashboard.
To serve previews, a workspace proxy needs its own wildcard access URL, HTTPS, and the same Coder release as the control plane.
The proxy's wildcard access URL must be on the same site as the dashboard, which means under the same registrable domain.
For example, a proxy can use `*.eu.coder.example.com` for a dashboard at `coder.example.com`.

If the selected proxy has no wildcard access URL, the preview tab shows `This region does not serve previews.`
If the proxy's wildcard access URL is on another site, the preview tab shows `Previews need a workspace proxy on the same site as the dashboard.`
To open the preview, the user can [select a workspace proxy](../../admin/networking/workspace-proxies.md#select-a-proxy) that meets these requirements, or the primary deployment.

## What it does

Only root chats outside [plan mode](../index.md#plan-mode) have the `open_preview` tool.
When the agent calls it, the tool result shows the workspace owner an **Open preview** button.
Other users who view the chat see `Only the workspace owner can open this preview.` instead.
A result for a workspace or agent that the chat doesn't use shows `This preview belongs to a different workspace.`

Nothing opens until the workspace owner selects the button.
The preview opens in a tab in the chat's right panel, and previews open nowhere else.
Selecting **Open preview** for a preview that already has a tab reloads that tab instead of opening another one, and a newer result also renames the tab.
If a user opens a preview's address directly in the browser, Coder shows an error page that says `Previews open only inside a Coder chat. Open this preview from its chat.`

Coder signs the browser in to a preview's address only when the browser has no valid sign-in for that address.
That happens the first time the user opens the preview in a browser session, and again after the sign-in expires.
Reloading or reopening the preview reuses the sign-in.

The agent can open two kinds of preview:

- A port preview shows a plain HTTP server that already listens on a workspace port.
  If the template sets `port_forwarding_helper = false` in the `display_apps` block of its `coder_agent` resource, the `open_preview` tool refuses port previews.
- A file preview shows a single `.html` file, or a directory that contains `index.html` (a directory preview).
  The workspace agent serves the files, so no server has to run.
  A preview of a single `.html` file serves only that file, so a page that loads local stylesheets, scripts, or images needs a directory preview.
  A file preview lasts until the workspace agent restarts, and then its tab shows `This preview is no longer available because the workspace agent restarted. Ask the agent to open it again.`
  When the agent opens the same path again, the preview keeps its address, and its existing tab works again.

## Preview security

In the user's browser, a preview runs scripts that the agent controls, signed in as that user.
Only the user who owns the workspace can open its previews.
Users with the Owner role and other administrators can't open another user's previews.
Users the workspace is shared with through [workspace sharing](../../workspaces/shared-workspaces.md) can't open its previews either, even when they can open the workspace's other apps.
If the chat's workspace belongs to another user, `open_preview` returns `Previews open only for the workspace owner, and this chat's workspace belongs to another user.`

Coder limits what a preview can do in the browser:

- The page loads scripts, styles, fonts, images, and other files only from the preview's own address.
  Inline scripts, inline styles, and `data:` and `blob:` URLs still work.
  The browser blocks files from a content delivery network (CDN) or another server, so bundle those files with the page.
- The page can't send `fetch`, `XMLHttpRequest`, WebSocket, or form requests to other addresses.
  Coder also refuses requests that a preview page sends to the user's other workspace apps, subdomain or path-based, when the control plane or workspace proxy serving the preview also serves those apps.
  Coder can't limit WebRTC connections, so scripts in a preview can still send data they can read, such as the preview's own files, to other hosts.
- Links to other addresses don't load, and links that open a new tab or window do nothing.
- No site other than Coder can show the preview in a frame.
- The page can't register a service worker.
- Coder drops each cookie that the preview's server sets with a `Domain` attribute, because the browser would send that cookie to the user's other workspace apps.

Workspace proxies serve previews with the same limits.

Coder can't stop scripts in a preview from setting cookies for a parent domain of the preview's address, such as the domain of the wildcard access URL.
The browser sends those cookies to the user's other workspace apps, and to Coder itself when the access URL is under the same domain.
`--host-prefix-cookie` keeps such a cookie from replacing the Coder session cookie, but a preview can still use one to overwrite a cookie that one of the user's other apps uses.

> [!WARNING]
> If the agent previews a directory that contains secrets, scripts in the preview can read them and send them to other hosts.
> A directory preview serves every regular file in the directory and its subdirectories, including files added after the agent opened it.
> It doesn't serve symbolic links, or a file or subdirectory whose name starts with a dot.
> Have the agent preview a dedicated build output directory, such as `dist`, that holds only what the page needs, and not a project root or a home directory.
