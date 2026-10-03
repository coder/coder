/// <reference lib="webworker" />

import type { WebpushMessage } from "#/api/typesGenerated";

declare const self: ServiceWorkerGlobalScope;

self.addEventListener("install", (_event) => {
	self.skipWaiting();
});

self.addEventListener("activate", (event) => {
	event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
	if (!event.data) {
		return;
	}

	let payload: WebpushMessage;
	try {
		payload = event.data?.json();
	} catch (e) {
		console.error("Error parsing push payload:", e);
		return;
	}

	event.waitUntil(
		self.clients
			.matchAll({ type: "window", includeUncontrolled: true })
			.then((clientList) => {
				// Only suppress if the user is actively viewing the
				// specific chat that triggered this notification and
				// the browser window is focused.
				const chatURL = payload.data?.url;
				if (chatURL) {
					const isVisible = clientList.some(
						(client) =>
							client.visibilityState === "visible" &&
							client.focused &&
							client.url.includes(chatURL),
					);
					if (isVisible) {
						return;
					}
				}
				return self.registration.showNotification(payload.title, {
					body: payload.body || "",
					icon: payload.icon || "/favicon.ico",
					data: payload.data,
					tag: payload.tag,
				});
			}),
	);
});

// Handle key rotation. The Push API spec requires the user agent to fire
// pushsubscriptionchange when the push service rotates the keys. Without
// this handler the device's local subscription updates but the server keeps
// the old keys, which is the failure mode after a PWA reinstall on iOS:
// the bell stays green client-side because pushManager.getSubscription()
// returns the new subscription, but coderd encrypts to the old keys and
// the device silently drops the message.
self.addEventListener("pushsubscriptionchange", (event) => {
	event.waitUntil(handlePushSubscriptionChange(event));
});

async function handlePushSubscriptionChange(
	event: PushSubscriptionChangeEvent,
): Promise<void> {
	// Prefer the application server key the browser already negotiated, so
	// we don't depend on the React side delivering a fresh VAPID key into
	// the worker. Fall back to the old subscription's options when the new
	// one is null (the browser revoked without auto-renewing).
	const applicationServerKey =
		event.newSubscription?.options.applicationServerKey ??
		event.oldSubscription?.options.applicationServerKey;
	if (!applicationServerKey) {
		return;
	}

	const subscription =
		event.newSubscription ??
		(await self.registration.pushManager.subscribe({
			userVisibleOnly: true,
			applicationServerKey,
		}));

	const json = subscription.toJSON();
	if (!json.endpoint || !json.keys) {
		return;
	}

	await fetch("/api/v2/users/me/webpush/subscription", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		credentials: "include",
		body: JSON.stringify({
			endpoint: json.endpoint,
			auth_key: json.keys.auth,
			p256dh_key: json.keys.p256dh,
		}),
	});

	if (event.oldSubscription) {
		await fetch("/api/v2/users/me/webpush/subscription", {
			method: "DELETE",
			headers: { "Content-Type": "application/json" },
			credentials: "include",
			body: JSON.stringify({ endpoint: event.oldSubscription.endpoint }),
		});
	}
}

// Handle notification click — navigate to the specific chat or agents page.
self.addEventListener("notificationclick", (event) => {
	event.notification.close();
	const targetUrl: string = event.notification.data?.url || "/agents";
	event.waitUntil(
		self.clients
			.matchAll({ type: "window", includeUncontrolled: true })
			.then((clientList) => {
				for (const client of clientList) {
					if (client.url.includes("/agents") && "focus" in client) {
						if (!client.url.includes(targetUrl)) {
							client.navigate(targetUrl);
						}
						return client.focus();
					}
				}
				return self.clients.openWindow(targetUrl);
			}),
	);
});
