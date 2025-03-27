/// <reference lib="webworker" />

import type { WebpushMessage } from "api/typesGenerated";

// @ts-ignore
declare const self: ServiceWorkerGlobalScope;

self.addEventListener("install", (event) => {
	self.skipWaiting();
});

self.addEventListener("activate", (event) => {
	event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
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

// Handle notification click
self.addEventListener("notificationclick", (event) => {
	event.notification.close();
});
