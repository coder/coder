// Service worker for handling desktop notifications
self.addEventListener('install', (event) => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

// Handle push event to show notifications
self.addEventListener('push', (event) => {
  let payload;
  try {
    payload = event.data.json();
  } catch (e) {
    payload = {
      title: 'New Notification',
      body: event.data ? event.data.text() : 'No payload'
    };
  }

  const title = payload.title || 'Coder Notification';
  const options = {
    body: payload.body || '',
    icon: '/favicon.ico',
    data: payload.data || {}
  };

  event.waitUntil(
    self.registration.showNotification(title, options)
  );
});

// Handle notification click
self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  // If a link is provided, navigate to it
  const data = event.notification.data;
  if (data && data.url) {
    event.waitUntil(
      clients.openWindow(data.url)
    );
  }
});