import { Injectable, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { ApiService } from './api.service';

@Injectable({
  providedIn: 'root'
})
export class PushService {
  private api = inject(ApiService);
  private checking = false;
  isSupported = signal<boolean>(false);
  isSubscribed = signal<boolean>(false);

  constructor() {
    if (typeof window !== 'undefined' && 'serviceWorker' in navigator && 'PushManager' in window) {
      this.isSupported.set(true);
      this.checkSubscription();
    }
  }

  async checkSubscription() {
    if (!this.isSupported() || this.checking) return;
    this.checking = true;
    try {
      let reg = await navigator.serviceWorker.getRegistration();
      if (!reg) {
        await navigator.serviceWorker.register('/sw.js');
        reg = await navigator.serviceWorker.ready;
      }

      let sub = await reg.pushManager.getSubscription();
      const res = await firstValueFrom(this.api.getVapidPublicKey());
      if (!res?.public_key) {
        this.isSubscribed.set(!!sub);
        return;
      }

      const expectedKey = this.urlBase64ToUint8Array(res.public_key);
      let oldEndpoint: string | undefined;

      if (sub) {
        const currentKeyBuf = sub.options?.applicationServerKey;
        if (currentKeyBuf && !this.keysMatch(new Uint8Array(currentKeyBuf), expectedKey)) {
          console.info('VAPID key mismatch detected; rotating push subscription.');
          oldEndpoint = sub.endpoint;
          await sub.unsubscribe();
          sub = null;
        }
      }

      if (!sub) {
        if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
          try {
            await navigator.serviceWorker.register('/sw.js');
            const readyReg = await navigator.serviceWorker.ready;
            sub = await readyReg.pushManager.subscribe({
              userVisibleOnly: true,
              applicationServerKey: expectedKey as any
            });
          } catch (err) {
            console.warn('Silent push re-subscribe requires user gesture:', err);
            this.isSubscribed.set(false);
            return;
          }
        } else {
          this.isSubscribed.set(false);
          return;
        }
      }

      this.isSubscribed.set(true);
      if (typeof localStorage !== 'undefined' && localStorage.getItem('screentime_token')) {
        await firstValueFrom(this.api.subscribePush(this.serializeSubscription(sub, oldEndpoint)));
      }
    } catch (e) {
      console.warn('Error checking push subscription:', e);
    } finally {
      this.checking = false;
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

      await navigator.serviceWorker.register('/sw.js');
      const reg = await navigator.serviceWorker.ready;

      const res = await firstValueFrom(this.api.getVapidPublicKey());
      if (!res?.public_key) return false;

      const expectedKey = this.urlBase64ToUint8Array(res.public_key);
      let sub = await reg.pushManager.getSubscription();
      let oldEndpoint: string | undefined;

      if (
        sub &&
        sub.options?.applicationServerKey &&
        !this.keysMatch(new Uint8Array(sub.options.applicationServerKey), expectedKey)
      ) {
        oldEndpoint = sub.endpoint;
        await sub.unsubscribe();
        sub = null;
      }

      if (!sub) {
        sub = await reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: expectedKey as any
        });
      }

      await firstValueFrom(this.api.subscribePush(this.serializeSubscription(sub, oldEndpoint)));
      this.isSubscribed.set(true);
      alert('Push notifications enabled! You will receive alerts when children request screen time.');
      return true;
    } catch (err) {
      console.error('Error subscribing to push:', err);
      return false;
    }
  }

  private keysMatch(a: Uint8Array, b: Uint8Array): boolean {
    return a.length === b.length && a.every((v, i) => v === b[i]);
  }

  private serializeSubscription(sub: PushSubscription, oldEndpoint?: string) {
    const p256dh = btoa(String.fromCharCode.apply(null, Array.from(new Uint8Array(sub.getKey('p256dh')!))));
    const auth = btoa(String.fromCharCode.apply(null, Array.from(new Uint8Array(sub.getKey('auth')!))));
    return {
      endpoint: sub.endpoint,
      p256dh,
      auth,
      ...(oldEndpoint ? { old_endpoint: oldEndpoint } : {})
    };
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

