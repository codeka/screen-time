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

  const payloadData = data.data || {};
  let actions = [];

  if (payloadData.request_id) {
    const maxActions = (self.Notification && self.Notification.maxActions) || 3;
    if (maxActions >= 4) {
      actions = [
        { action: 'grant_15', title: '+15 Min' },
        { action: 'grant_30', title: '+30 Min' },
        { action: 'custom', title: 'Custom…' },
        { action: 'deny', title: 'Deny' }
      ];
    } else if (maxActions === 3) {
      actions = [
        { action: 'grant_30', title: '+30 Min' },
        { action: 'custom', title: 'Custom…' },
        { action: 'deny', title: 'Deny' }
      ];
    } else {
      actions = [
        { action: 'grant_30', title: '+30 Min' },
        { action: 'custom', title: 'Custom…' }
      ];
    }
  }

  const options = {
    body: data.body,
    icon: '/favicon.ico',
    badge: '/favicon.ico',
    tag: data.tag || 'screentime-request',
    renotify: true,
    requireInteraction: !!payloadData.request_id,
    data: payloadData,
    actions
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

    const headers = { 'Content-Type': 'application/json' };
    if (data.action_token) {
      headers['Authorization'] = `Bearer ${data.action_token}`;
    }

    if (action === 'custom' && data.request_id) {
      const targetUrl = `/?custom_request=${encodeURIComponent(data.request_id)}`;
      event.waitUntil(
        self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientList) => {
          for (const client of clientList) {
            client.postMessage({
              type: 'OPEN_CUSTOM_GRANT',
              requestId: data.request_id,
              deviceId: data.device_id,
              childName: data.child_name,
              deviceName: data.device_name
            });
            if ('focus' in client) {
              return client.focus();
            }
          }
          if (self.clients.openWindow) {
            return self.clients.openWindow(targetUrl);
          }
        })
      );
      return;
    }

    if (action && action.startsWith('grant_') && data.request_id) {
      const minutes = parseInt(action.replace('grant_', ''), 10);
      event.waitUntil(
        fetch(`/api/requests/${data.request_id}/resolve`, {
          method: 'POST',
          headers,
          body: JSON.stringify({ action: 'GRANT', grant_minutes: minutes })
        })
          .then(() =>
            self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
              clients.forEach((c) => c.postMessage({ type: 'REFRESH_DASHBOARD' }));
            })
          )
          .catch(() => { })
      );
      return;
    }

    if (action === 'deny' && data.request_id) {
      event.waitUntil(
        fetch(`/api/requests/${data.request_id}/resolve`, {
          method: 'POST',
          headers,
          body: JSON.stringify({ action: 'DENY' })
        })
          .then(() =>
            self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
              clients.forEach((c) => c.postMessage({ type: 'REFRESH_DASHBOARD' }));
            })
          )
          .catch(() => { })
      );
      return;
    }

    // Clicked notification body directly: open/focus dashboard (if it's a request, highlight/open it)
    const defaultUrl = data.request_id ? `/?custom_request=${encodeURIComponent(data.request_id)}` : '/';
    event.waitUntil(
      self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientList) => {
        for (const client of clientList) {
          if (data.request_id) {
            client.postMessage({ type: 'REFRESH_DASHBOARD' });
          }
          if ('focus' in client) {
            return client.focus();
          }
        }
        if (self.clients.openWindow) {
          return self.clients.openWindow(defaultUrl);
        }
      })
    );
  });
