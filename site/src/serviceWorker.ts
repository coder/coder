/// <reference lib="webworker" />

import type { WebpushMessage } from "api/typesGenerated";

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
