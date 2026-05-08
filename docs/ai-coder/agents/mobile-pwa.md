# Coder Agents on mobile

Coder Agents ships as a Progressive Web App (PWA) that you can install on your
phone. Once installed, it opens as a standalone app with its own icon, push
notifications, and a full-screen interface without browser chrome.

The PWA launches directly to the `/agents` page, so you can start or monitor
coding agents from anywhere.

> [!NOTE]
> Coder Agents is in Beta. The mobile experience works today but is
> under active development. Some UI elements are optimized for desktop
> viewports.

## Prerequisites

- **HTTPS access URL.** Your Coder deployment must be accessible over HTTPS.
  Push notifications require a secure origin, and mobile browsers will not
  offer the install prompt on plain HTTP.
- **Coder Agents User role.** You must have the **Coder Agents User** role
  assigned in your organization. See
  [Getting Started](./getting-started.md#step-2-grant-coder-agents-user)
  for details.
- **Supported browser.** Use Safari on iOS or Chrome on Android.

## Install on iOS

1. Open **Safari** and navigate to your Coder deployment URL
   (e.g., `https://coder.example.com/agents`).
1. Tap the **Share** button (the square with an upward arrow) in the bottom
   toolbar.
1. Scroll down and tap **Add to Home Screen**.
1. Optionally edit the name. The default is **Agents**.
1. Tap **Add** in the upper right.

The Coder Agents icon appears on your home screen. Tapping it opens the app in
standalone mode with no Safari UI.

> [!TIP]
> If you do not see **Add to Home Screen** in the share sheet, scroll the
> bottom row of actions. It may also appear under **More** depending on your
> iOS version.

## Install on Android

1. Open **Chrome** and navigate to your Coder deployment URL
   (e.g., `https://coder.example.com/agents`).
1. Chrome may show an **Install app** banner at the bottom of the screen. If
   it does, tap **Install** and skip to step 5.
1. If no banner appears, tap the **three-dot menu** in the upper right.
1. Tap **Add to Home screen** or **Install app** (the wording varies by Chrome
   version).
1. Confirm by tapping **Install** or **Add**.

The app appears in your app drawer and home screen. It launches in standalone
mode without browser controls.

## Enable push notifications

Push notifications alert you when an agent finishes a task, encounters an
error, or needs your input. This is especially useful on mobile where you may
not be actively watching the chat.

1. Open a chat in Coder Agents (either in the PWA or browser).
1. Tap the **bell icon** in the chat header.
1. Your browser prompts you to allow notifications. Tap **Allow**.

When notifications are active, the bell icon turns **green**. Tap it again to
unsubscribe.

> [!NOTE]
> Notifications are suppressed when you are actively viewing the specific
> chat that triggered them. You only receive push notifications for chats
> you are not currently looking at.

## Known behaviors

### Session persistence

The PWA uses persistent cookies so you stay logged in even when the OS kills
the app process in the background. You should not need to re-authenticate
each time you open the app.

### iOS push notification reliability

iOS may occasionally rotate push subscription keys when the PWA is
reinstalled or updated. Coder handles this automatically by re-registering
the subscription with the server. If you stop receiving notifications after
an iOS update, toggle the bell icon off and on again to force a fresh
subscription.

### Viewport behavior

The PWA disables pinch-to-zoom to prevent accidental zooming while
interacting with the chat interface. Standard scrolling works normally.

## Next steps

- [Getting Started](./getting-started.md) for initial Coder Agents setup.
- [Architecture](./architecture.md) for how the agent loop works.
- [Models](./models.md) to configure LLM providers.
