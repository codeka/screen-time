import { Injectable, inject, signal } from '@angular/core';
import { AuthService } from './auth.service';
import { Subject } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class RealtimeService {
  private auth = inject(AuthService);
  private ws: WebSocket | null = null;
  private reconnectTimer: any = null;

  isConnected = signal<boolean>(false);
  events$ = new Subject<any>();

  constructor() {
    // If token exists, connect
    if (this.auth.token()) {
      this.connect();
    }
  }

  connect() {
    const token = this.auth.token();
    if (!token || typeof window === 'undefined') return;

    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let host = window.location.host;
    if (window.location.port === '4200') {
      host = 'localhost:8080';
    }

    const url = `${protocol}//${host}/api/ws/admin?token=${encodeURIComponent(token)}`;

    try {
      this.ws = new WebSocket(url);

      this.ws.onopen = () => {
        this.isConnected.set(true);
        clearTimeout(this.reconnectTimer);
      };

      this.ws.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          this.events$.next(payload);
        } catch (e) {
          console.error('Failed to parse WS message:', e);
        }
      };

      this.ws.onclose = () => {
        this.isConnected.set(false);
        this.scheduleReconnect();
      };

      this.ws.onerror = () => {
        this.ws?.close();
      };
    } catch (e) {
      this.scheduleReconnect();
    }
  }

  disconnect() {
    clearTimeout(this.reconnectTimer);
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.isConnected.set(false);
  }

  private scheduleReconnect() {
    clearTimeout(this.reconnectTimer);
    this.reconnectTimer = setTimeout(() => {
      if (this.auth.isLoggedIn()) {
        this.connect();
      }
    }, 4000);
  }
}
