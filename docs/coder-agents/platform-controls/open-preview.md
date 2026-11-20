---
title: Workspace previews
---

> [!NOTE]
> Workspace previews are experimental.
> The `open_preview` tool and the way previews behave can change in any release.

Workspace previews let the agent in a chat show a web page from the workspace in a tab in the chat's right panel.
This page is for Coder deployment administrators who turn on workspace previews.

## Enable workspace previews

Workspace previews require a [wildcard access URL](../../admin/networking/wildcard-access-url.md).
Without one, `open_preview` returns an error.

Previews also require HTTPS on the access URL and on every [workspace proxy](../../admin/networking/workspace-proxies.md) that serves previews.
Over plain HTTP, previews don't load.

To give the agent the tool, add `open-preview` to the experiments that the control plane enables:

```sh
CODER_EXPERIMENTS=open-preview
```

The `*` value doesn't enable `open-preview`.
Refer to [Feature stages](../../reference/feature-stages.md) for how experiments work.
Restart the control plane after you change the experiment list.
While the experiment is off, the agent doesn't get the tool and Coder doesn't serve previews.

## What it does

Only root chats outside [plan mode](../index.md#plan-mode) have the `open_preview` tool.
When the agent calls it, the tool result shows an **Open preview** button.
Nothing opens until the user selects the button.

The agent can open two kinds of preview:

- A port preview shows a plain HTTP server that already listens on a workspace port.
  If the template sets `port_forwarding_helper = false` in the `display_apps` block of its `coder_agent` resource, Coder refuses port previews.
- A file preview shows an `.html` file, or a directory that contains `index.html`.
  The workspace agent serves the files, so no server has to run.
  A preview of an `.html` file serves only that file, so a page that loads local stylesheets, scripts, or images needs a directory preview.

## Preview security

In the user's browser, a preview runs scripts that the agent controls, signed in as that user.
Only the workspace owner can open previews.
Users the workspace is shared with through [workspace sharing](../../workspaces/shared-workspaces.md), and users with the Owner role, can't open them.

Coder limits what a preview can do in the browser:

- The page loads scripts, styles, fonts, images, and other files only from the preview's own address.
  Inline scripts, inline styles, and `data:` and `blob:` URLs still work.
  The browser blocks files from a content delivery network (CDN) or another server, so bundle those files with the page.
- The page sends requests only to the preview's own address.
  Coder also refuses requests that a preview page sends to the user's other workspace apps.
- Links to other addresses don't load, and links that open a new tab or window do nothing.
- No site other than Coder can show the preview in a frame.
- The page can't register a service worker.
- Coder drops each cookie that the preview's server sets with a `Domain` attribute, because the browser would send that cookie to the user's other workspace apps.

Workspace proxies serve previews with the same limits.

Coder can't stop scripts in a preview from setting cookies for a parent domain of the preview's address, such as the domain of the wildcard access URL.
The browser sends those cookies to the user's other workspace apps, and to Coder itself when the access URL is under the same domain.
A preview can use such a cookie to overwrite a cookie that one of the user's other apps uses.

> [!WARNING]
> If the agent previews a directory that contains secrets, scripts in the preview can read them.
> A directory preview serves every regular file in the directory and its subdirectories, including files added after the agent opened it.
> It doesn't serve symbolic links, or a file or subdirectory whose name starts with a dot.
