import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { AuthService } from '../../services/auth.service';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [CommonModule, FormsModule],
  template: `
    <div class="auth-container">
      <div class="auth-card">
        <div class="auth-header">
          <div class="app-icon">⏱️</div>
          <h1>Screen Time Admin</h1>
          <p class="subtitle">Unified parent portal for Windows & Android</p>
        </div>

        <div class="tabs">
          <button 
            type="button" 
            [class.active]="mode() === 'login'" 
            (click)="mode.set('login')">
            Sign In
          </button>
          <button 
            type="button" 
            [class.active]="mode() === 'register'" 
            (click)="mode.set('register')">
            Create Family
          </button>
        </div>

        @if (error()) {
          <div class="alert-error">
            {{ error() }}
          </div>
        }

        @if (mode() === 'login') {
          <form (ngSubmit)="onLogin()">
            <div class="form-group">
              <label>Parent Email</label>
              <input 
                type="email" 
                [(ngModel)]="loginEmail" 
                name="email" 
                placeholder="parent@example.com" 
                required />
            </div>

            <div class="form-group">
              <label>Password</label>
              <input 
                type="password" 
                [(ngModel)]="loginPassword" 
                name="password" 
                placeholder="••••••••" 
                required />
            </div>

            <button type="submit" class="btn-primary" [disabled]="loading()">
              {{ loading() ? 'Signing in...' : 'Sign In' }}
            </button>
          </form>
        } @else {
          <form (ngSubmit)="onRegister()">
            <div class="form-group">
              <label>Family Name</label>
              <input 
                type="text" 
                [(ngModel)]="regFamilyName" 
                name="familyName" 
                placeholder="The Miller Family" 
                required />
            </div>

            <div class="form-group">
              <label>Your Display Name</label>
              <input 
                type="text" 
                [(ngModel)]="regDisplayName" 
                name="displayName" 
                placeholder="Mom / Dad" 
                required />
            </div>

            <div class="form-group">
              <label>Email</label>
              <input 
                type="email" 
                [(ngModel)]="regEmail" 
                name="email" 
                placeholder="parent@example.com" 
                required />
            </div>

            <div class="form-group">
              <label>Password</label>
              <input 
                type="password" 
                [(ngModel)]="regPassword" 
                name="password" 
                placeholder="Minimum 8 characters" 
                required />
            </div>

            <button type="submit" class="btn-primary" [disabled]="loading()">
              {{ loading() ? 'Creating Family...' : 'Create Family Account' }}
            </button>
          </form>
        }
      </div>
    </div>
  `,
  styles: [`
    .auth-container {
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1.5rem;
      background: radial-gradient(circle at top, #1e1b4b, #0f172a);
    }
    .auth-card {
      width: 100%;
      max-width: 420px;
      background: rgba(30, 41, 59, 0.85);
      backdrop-filter: blur(12px);
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 1.25rem;
      padding: 2rem;
      box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    .auth-header {
      text-align: center;
      margin-bottom: 1.75rem;
    }
    .app-icon {
      font-size: 2.75rem;
      margin-bottom: 0.5rem;
    }
    h1 {
      font-size: 1.5rem;
      font-weight: 700;
      color: #f8fafc;
      margin: 0;
    }
    .subtitle {
      color: #94a3b8;
      font-size: 0.875rem;
      margin-top: 0.25rem;
    }
    .tabs {
      display: flex;
      background: #0f172a;
      border-radius: 0.75rem;
      padding: 0.25rem;
      margin-bottom: 1.5rem;
    }
    .tabs button {
      flex: 1;
      padding: 0.625rem;
      border: none;
      background: transparent;
      color: #94a3b8;
      font-weight: 600;
      font-size: 0.875rem;
      border-radius: 0.5rem;
      cursor: pointer;
      transition: all 0.2s;
    }
    .tabs button.active {
      background: #4f46e5;
      color: #ffffff;
      box-shadow: 0 2px 8px rgba(79, 70, 229, 0.4);
    }
    .alert-error {
      background: rgba(239, 68, 68, 0.15);
      border: 1px solid rgba(239, 68, 68, 0.3);
      color: #fca5a5;
      padding: 0.75rem;
      border-radius: 0.5rem;
      margin-bottom: 1rem;
      font-size: 0.875rem;
    }
    .form-group {
      margin-bottom: 1.25rem;
    }
    label {
      display: block;
      font-size: 0.8125rem;
      font-weight: 500;
      color: #cbd5e1;
      margin-bottom: 0.375rem;
    }
    input {
      width: 100%;
      padding: 0.75rem 1rem;
      background: #0f172a;
      border: 1px solid #334155;
      border-radius: 0.625rem;
      color: #f8fafc;
      font-size: 0.9375rem;
      box-sizing: border-box;
      transition: border-color 0.2s;
    }
    input:focus {
      outline: none;
      border-color: #6366f1;
      box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
    }
    .btn-primary {
      width: 100%;
      padding: 0.875rem;
      background: #4f46e5;
      color: white;
      border: none;
      border-radius: 0.625rem;
      font-size: 0.9375rem;
      font-weight: 600;
      cursor: pointer;
      margin-top: 0.5rem;
      transition: background 0.2s;
    }
    .btn-primary:hover:not(:disabled) {
      background: #4338ca;
    }
    .btn-primary:disabled {
      opacity: 0.6;
      cursor: not-allowed;
    }
  `]
})
export class LoginComponent {
  private auth = inject(AuthService);
  private router = inject(Router);

  mode = signal<'login' | 'register'>('login');
  loading = signal<boolean>(false);
  error = signal<string>('');

  loginEmail = '';
  loginPassword = '';

  regFamilyName = '';
  regDisplayName = '';
  regEmail = '';
  regPassword = '';

  onLogin() {
    this.error.set('');
    this.loading.set(true);
    this.auth.login({ email: this.loginEmail, password: this.loginPassword }).subscribe({
      next: () => {
        this.loading.set(false);
        this.router.navigate(['/']);
      },
      error: (err) => {
        this.loading.set(false);
        this.error.set(err.error?.error || 'Invalid email or password.');
      }
    });
  }

  onRegister() {
    this.error.set('');
    this.loading.set(true);
    this.auth.register({
      family_name: this.regFamilyName,
      display_name: this.regDisplayName,
      email: this.regEmail,
      password: this.regPassword
    }).subscribe({
      next: () => {
        this.loading.set(false);
        this.router.navigate(['/']);
      },
      error: (err) => {
        this.loading.set(false);
        this.error.set(err.error?.error || 'Registration failed.');
      }
    });
  }
}

