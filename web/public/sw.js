// Service Worker for Screen Time Admin PWA
self.addEventListener('install', (event) => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('push', (event) => {
  let data = {
    title: 'Screen Time Alert',
    body: 'A managed device has updated status.',
    tag: 'screentime-general',
    data: {}
  };

  if (event.data) {
    try {
      data = event.data.json();
    } catch (e) {
      data.body = event.data.text();
    }
  }

  const options = {
    body: data.body,
    icon: '/favicon.ico',
    badge: '/favicon.ico',
    tag: data.tag || 'screentime-request',
    renotify: true,
    requireInteraction: true,
    data: data.data || {},
    actions: [
      { action: 'grant_15', title: '+15 Min' },
      { action: 'grant_30', title: '+30 Min' },
      { action: 'grant_60', title: '+1 Hour' },
      { action: 'deny', title: 'Deny' }
    ]
  };

  event.waitUntil(
    self.registration.showNotification(data.title, options)
  );
});

self.addEventListener('notificationclick', (event) => {
  const notification = event.notification;
  const action = event.action;
  const data = notification.data || {};
  notification.close();

  if (action && action.startsWith('grant_')) {
    const minutes = parseInt(action.replace('grant_', ''), 10);
    if (data.request_id) {
      event.waitUntil(
        fetch(`/api/requests/${data.request_id}/resolve`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action: 'GRANT', grant_minutes: minutes })
        }).catch(() => {})
      );
      return;
    }
  } else if (action === 'deny') {
    if (data.request_id) {
      event.waitUntil(
        fetch(`/api/requests/${data.request_id}/resolve`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action: 'DENY' })
        }).catch(() => {})
      );
      return;
    }
  }

  // If clicked directly without action button, focus or open window
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientList) => {
      for (const client of clientList) {
        if ('focus' in client) {
          return client.focus();
        }
      }
      if (self.clients.openWindow) {
        return self.clients.openWindow('/');
      }
    })
  );
});
