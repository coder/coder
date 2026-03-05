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
			data: payload.data,
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
						client.navigate(targetUrl);
						return client.focus();
					}
				}
				return self.clients.openWindow(targetUrl);
			}),
	);
});
