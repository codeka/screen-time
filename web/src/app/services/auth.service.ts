import { Injectable, signal, computed, inject } from '@angular/core';
import { Router } from '@angular/router';
import { ApiService } from './api.service';
import { FamilyUser } from '../models/types';
import { tap } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class AuthService {
  private api = inject(ApiService);
  private router = inject(Router);

  token = signal<string | null>(this.getStoredToken());
  currentUser = signal<FamilyUser | null>(null);
  familyName = signal<string>('');

  isLoggedIn = computed(() => !!this.token());

  constructor() {
    if (this.token()) {
      this.refreshMe();
    }
  }

  private getStoredToken(): string | null {
    if (typeof window !== 'undefined') {
      return localStorage.getItem('screentime_token');
    }
    return null;
  }

  login(credentials: { email: string; password: string }) {
    return this.api.login(credentials).pipe(
      tap((res) => {
        localStorage.setItem('screentime_token', res.token);
        this.token.set(res.token);
        this.currentUser.set(res.user);
        this.refreshMe();
      })
    );
  }

  register(data: { family_name: string; display_name: string; email: string; password: string }) {
    return this.api.register(data).pipe(
      tap((res) => {
        localStorage.setItem('screentime_token', res.token);
        this.token.set(res.token);
        this.currentUser.set(res.user);
        this.refreshMe();
      })
    );
  }

  refreshMe() {
    this.api.getMe().subscribe({
      next: (res) => {
        this.currentUser.set(res.user);
        this.familyName.set(res.family_name);
      },
      error: () => {
        this.logout();
      }
    });
  }

  logout() {
    localStorage.removeItem('screentime_token');
    this.token.set(null);
    this.currentUser.set(null);
    this.familyName.set('');
    this.router.navigate(['/login']);
  }
}

