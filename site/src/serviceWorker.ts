/// <reference lib="webworker" />

import { PushNotification, PushNotificationAction } from "api/typesGenerated";

// @ts-ignore
declare const self: ServiceWorkerGlobalScope;

self.addEventListener('install', (event) => {
    self.skipWaiting();
});

self.addEventListener('activate', (event) => {
    event.waitUntil(self.clients.claim());
});

self.addEventListener('push', (event) => {
    if (!event.data) {
        return;
    }

    let payload: PushNotification;
    try {
        payload = event.data.json();
    } catch (e) {
        return;
    }

    console.log("PAYLOAD", payload);

    event.waitUntil(
        self.registration.showNotification(payload.title, {
            body: payload.body || "",
            icon: payload.icon || "/favicon.ico",
            // actions: payload.actions.map((action: PushNotificationAction) => ({
            //     title: action.title,
            //     action: action.url,
            // })) || [],
        })
    );
});

// Handle notification click
self.addEventListener('notificationclick', (event) => {
    event.notification.close();

    // If a link is provided, navigate to it
    const data = event.notification.data;
    // if (data && data.url) {
    //     event.waitUntil(
    //         clients.openWindow(data.url)
    //     );
    // }
});
