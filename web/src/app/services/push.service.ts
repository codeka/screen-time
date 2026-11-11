import { Injectable, inject, signal } from '@angular/core';
import { ApiService } from './api.service';

@Injectable({
  providedIn: 'root'
})
export class PushService {
  private api = inject(ApiService);
  isSupported = signal<boolean>(false);
  isSubscribed = signal<boolean>(false);

  constructor() {
    if (typeof window !== 'undefined' && 'serviceWorker' in navigator && 'PushManager' in window) {
      this.isSupported.set(true);
      this.checkSubscription();
    }
  }

  async checkSubscription() {
    try {
      const reg = await navigator.serviceWorker.getRegistration();
      if (reg) {
        const sub = await reg.pushManager.getSubscription();
        this.isSubscribed.set(!!sub);
      }
    } catch (e) {
      console.warn('Error checking push subscription:', e);
    }
  }

  async subscribe(): Promise<boolean> {
    if (!this.isSupported()) return false;

    try {
      const permission = await Notification.requestPermission();
      if (permission !== 'granted') {
        alert('Notification permission was not granted.');
        return false;
      }

      const reg = await navigator.serviceWorker.register('/sw.js');
      await navigator.serviceWorker.ready;

      this.api.getVapidPublicKey().subscribe({
        next: async (res) => {
          if (!res.public_key) return;

          const applicationServerKey = this.urlBase64ToUint8Array(res.public_key);
          const sub = await reg.pushManager.subscribe({
            userVisibleOnly: true,
            applicationServerKey: applicationServerKey as any
          });

          const p256dh = btoa(String.fromCharCode.apply(null, Array.from(new Uint8Array(sub.getKey('p256dh')!))));
          const auth = btoa(String.fromCharCode.apply(null, Array.from(new Uint8Array(sub.getKey('auth')!))));

          this.api.subscribePush({
            endpoint: sub.endpoint,
            p256dh: p256dh,
            auth: auth
          }).subscribe({
            next: () => {
              this.isSubscribed.set(true);
              alert('Push notifications enabled! You will receive alerts when children request screen time.');
            },
            error: (err) => console.error('Failed to save subscription:', err)
          });
        },
        error: (err) => console.error('Failed to get VAPID key:', err)
      });

      return true;
    } catch (err) {
      console.error('Error subscribing to push:', err);
      return false;
    }
  }

  private urlBase64ToUint8Array(base64String: string): Uint8Array {
    const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
    const base64 = (base64String + padding).replace(/\-/g, '+').replace(/_/g, '/');
    const rawData = window.atob(base64);
    const outputArray = new Uint8Array(rawData.length);
    for (let i = 0; i < rawData.length; ++i) {
      outputArray[i] = rawData.charCodeAt(i);
    }
    return outputArray;
  }
}

