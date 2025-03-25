/// <reference lib="webworker" />

import {
	type PushNotification,
	PushNotificationAction,
} from "api/typesGenerated";

// @ts-ignore
declare const self: ServiceWorkerGlobalScope;

self.addEventListener("install", (event) => {
	self.skipWaiting();
});

self.addEventListener("activate", (event) => {
	event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
	if (!event.data) {
		return;
	}

	let payload: PushNotification;
	try {
		payload = event.data.json();
	} catch (e) {
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
