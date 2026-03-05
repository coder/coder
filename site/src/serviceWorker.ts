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
		self.registration.showNotification(payload.title, {
			body: payload.body || "",
			icon: payload.icon || "/favicon.ico",
		}),
	);
});

// Handle notification click — navigate to the agents page
self.addEventListener("notificationclick", (event) => {
	event.notification.close();
	event.waitUntil(
		self.clients
			.matchAll({ type: "window", includeUncontrolled: true })
			.then((clientList) => {
				for (const client of clientList) {
					if (client.url.includes("/agents") && "focus" in client) {
						return client.focus();
					}
				}
				return self.clients.openWindow("/agents");
			}),
	);
});
