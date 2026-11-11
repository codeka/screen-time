import { Component, OnInit, OnDestroy, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ApiService } from '../../services/api.service';
import { AuthService } from '../../services/auth.service';
import { PushService } from '../../services/push.service';
import { RealtimeService } from '../../services/realtime.service';
import { ChildProfile, ScheduleRule, ScreenTimeRequest, Device } from '../../models/types';
import { Subscription } from 'rxjs';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [CommonModule, FormsModule],
  template: `
    <div class="dashboard-layout">
      <!-- Navbar -->
      <header class="navbar">
        <div class="nav-left">
          <div class="logo-badge">⏱️</div>
          <div>
            <h2 class="family-title">{{ auth.familyName() || 'Family Portal' }}</h2>
            <div class="user-subtitle">Logged in as {{ auth.currentUser()?.display_name }} (Parent)</div>
          </div>
        </div>

        <div class="nav-right">
          <div class="status-pill" [class.connected]="realtime.isConnected()">
            <span class="dot"></span>
            {{ realtime.isConnected() ? 'Live' : 'Connecting' }}
          </div>

          @if (push.isSupported() && !push.isSubscribed()) {
            <button class="btn-push" (click)="enablePush()">
              🔔 Enable Push
            </button>
          }

          <button class="btn-logout" (click)="auth.logout()">Sign Out</button>
        </div>
      </header>

      <main class="main-content">
        <!-- Pending Screen Time Requests Alert Banner -->
        @if (pendingRequests().length > 0) {
          <section class="requests-section">
            <h3 class="section-title text-amber">
              ⚡ Action Required: Screen Time Requests ({{ pendingRequests().length }})
            </h3>
            
            <div class="requests-grid">
              @for (req of pendingRequests(); track req.id) {
                <div class="request-card">
                  <div class="request-header">
                    <div>
                      <div class="request-child-name">{{ req.child_display_name }}</div>
                      <div class="request-device-name">on {{ req.device_name }}</div>
                    </div>
                    <span class="badge-pending">Pending</span>
                  </div>

                  <p class="request-msg">Child is asking to unlock their device.</p>

                  <div class="grant-actions">
                    <button class="btn-grant" (click)="resolve(req.id, 'GRANT', 15)">+15 min</button>
                    <button class="btn-grant" (click)="resolve(req.id, 'GRANT', 30)">+30 min</button>
                    <button class="btn-grant primary" (click)="resolve(req.id, 'GRANT', 45)">+45 min</button>
                    <button class="btn-grant" (click)="resolve(req.id, 'GRANT', 60)">+1 hr</button>
                    <button class="btn-grant" (click)="resolve(req.id, 'GRANT', 120)">+2 hr</button>
                    <button class="btn-deny" (click)="resolve(req.id, 'DENY')">Deny</button>
                  </div>
                </div>
              }
            </div>
          </section>
        }

        <!-- Children Management Section -->
        <section class="children-section">
          <div class="section-header">
            <div>
              <h3 class="section-title">Children & Managed Devices</h3>
              <p class="section-subtitle">Manage screen allowances, pairing, and weekly schedules</p>
            </div>
            <button class="btn-primary-sm" (click)="showAddChildModal.set(true)">
              + Add Child
            </button>
          </div>

          @if (loading()) {
            <div class="loading-state">Loading family profiles...</div>
          } @else if (children().length === 0) {
            <div class="empty-state">
              <p>No children added yet. Add a child to generate pairing codes for Windows and Android.</p>
              <button class="btn-primary-sm" (click)="showAddChildModal.set(true)">Add First Child</button>
            </div>
          } @else {
            <div class="children-grid">
              @for (child of children(); track child.id) {
                <div class="child-card">
                  <div class="child-header">
                    <div>
                      <h4 class="child-name">{{ child.display_name }}</h4>
                      <div class="emergency-tag">
                        📞 Emergency: {{ child.emergency_contact || 'None set' }}
                      </div>
                    </div>

                    <div class="child-menu-btns">
                      <button class="btn-icon" title="Weekly Schedule" (click)="openScheduleModal(child)">
                        📅 Schedule
                      </button>
                      <button class="btn-icon" title="Pair New Device" (click)="openPairingModal(child)">
                        📱 Pair Device
                      </button>
                    </div>
                  </div>

                  <!-- Devices list -->
                  <div class="devices-list">
                    <div class="devices-label">Connected Devices:</div>
                    @if (!child.devices || child.devices.length === 0) {
                      <div class="no-devices">
                        No paired devices yet. Click "Pair Device" above.
                      </div>
                    } @else {
                      @for (dev of child.devices; track dev.id) {
                        <div class="device-row">
                          <div class="device-info">
                            <span class="platform-icon">
                              {{ dev.platform === 'WINDOWS' ? '💻' : '📱' }}
                            </span>
                            <div>
                              <span class="device-name">{{ dev.device_name }}</span>
                              <span class="device-platform">({{ dev.platform }})</span>
                            </div>
                          </div>

                          <span class="device-badge" [ngClass]="dev.status.toLowerCase()">
                            {{ dev.status }}
                          </span>
                        </div>
                      }
                    }
                  </div>

                  <!-- Active schedule summary -->
                  <div class="schedule-summary">
                    <span class="sched-icon">⏰</span>
                    <span class="sched-text">
                      {{ getScheduleSummary(child) }}
                    </span>
                  </div>
                </div>
              }
            </div>
          }
        </section>
      </main>

      <!-- Add Child Modal -->
      @if (showAddChildModal()) {
        <div class="modal-backdrop">
          <div class="modal-card">
            <h3>Add Child Profile</h3>
            <div class="form-group">
              <label>Child's Name</label>
              <input type="text" [(ngModel)]="newChildName" placeholder="e.g. Lucas" />
            </div>
            <div class="form-group">
              <label>Emergency Contact Phone (Optional)</label>
              <input type="text" [(ngModel)]="newChildEmergency" placeholder="e.g. 555-0199" />
            </div>
            <div class="modal-actions">
              <button class="btn-secondary" (click)="showAddChildModal.set(false)">Cancel</button>
              <button class="btn-primary" (click)="createChild()">Create Profile</button>
            </div>
          </div>
        </div>
      }

      <!-- Pairing Code Modal -->
      @if (activePairingChild()) {
        <div class="modal-backdrop">
          <div class="modal-card text-center">
            <h3>Pair Device for {{ activePairingChild()?.display_name }}</h3>
            <p class="modal-subtitle">
              Install the app on Windows or Android and enter this pairing code:
            </p>

            <div class="pairing-code-box">
              {{ currentPairingCode() || 'Generating...' }}
            </div>

            <p class="code-timer">Code valid for 15 minutes</p>

            <div class="pairing-instructions">
              <ol>
                <li>Launch the Screen Time app on the child's device.</li>
                <li>Type the 6-character code above.</li>
                <li>Device will automatically link and lock.</li>
              </ol>
            </div>

            <div class="modal-actions center">
              <button class="btn-primary" (click)="activePairingChild.set(null)">Done</button>
            </div>
          </div>
        </div>
      }

      <!-- Weekly Schedule Modal -->
      @if (activeScheduleChild()) {
        <div class="modal-backdrop">
          <div class="modal-card wide">
            <div class="modal-header-between">
              <div>
                <h3>Weekly Recurring Schedule</h3>
                <p class="modal-subtitle">
                  Set unrestricted times for <strong>{{ activeScheduleChild()?.display_name }}</strong>. During these windows, the device never shows the lock screen.
                </p>
              </div>
              <button class="btn-close" (click)="activeScheduleChild.set(null)">✕</button>
            </div>

            <div class="schedule-matrix">
              @for (day of daysOfWeek; track day.index) {
                <div class="schedule-row" [class.enabled]="scheduleState[day.index].enabled">
                  <div class="day-col">
                    <label class="toggle-switch">
                      <input 
                        type="checkbox" 
                        [(ngModel)]="scheduleState[day.index].enabled" />
                      <span class="slider"></span>
                    </label>
                    <span class="day-name">{{ day.name }}</span>
                  </div>

                  <div class="time-col" [class.disabled]="!scheduleState[day.index].enabled">
                    <label>From</label>
                    <input 
                      type="time" 
                      [(ngModel)]="scheduleState[day.index].start_time" 
                      [disabled]="!scheduleState[day.index].enabled" />
                    
                    <label>To</label>
                    <input 
                      type="time" 
                      [(ngModel)]="scheduleState[day.index].end_time" 
                      [disabled]="!scheduleState[day.index].enabled" />
                  </div>
                </div>
              }
            </div>

            <div class="modal-actions">
              <button class="btn-secondary" (click)="activeScheduleChild.set(null)">Cancel</button>
              <button class="btn-primary" (click)="saveScheduleRules()">Save Schedule</button>
            </div>
          </div>
        </div>
      }
    </div>
  `,
  styles: [`
    .dashboard-layout {
      min-height: 100vh;
      background: #0f172a;
      color: #f8fafc;
    }
    .navbar {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 1rem 1.5rem;
      background: #1e293b;
      border-bottom: 1px solid #334155;
    }
    .nav-left {
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }
    .logo-badge {
      font-size: 1.75rem;
    }
    .family-title {
      font-size: 1.125rem;
      font-weight: 700;
      margin: 0;
    }
    .user-subtitle {
      font-size: 0.75rem;
      color: #94a3b8;
    }
    .nav-right {
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }
    .status-pill {
      display: flex;
      align-items: center;
      gap: 0.375rem;
      font-size: 0.75rem;
      font-weight: 600;
      padding: 0.25rem 0.625rem;
      border-radius: 9999px;
      background: rgba(239, 68, 68, 0.2);
      color: #fca5a5;
    }
    .status-pill.connected {
      background: rgba(34, 197, 94, 0.2);
      color: #86efac;
    }
    .dot {
      width: 0.5rem;
      height: 0.5rem;
      border-radius: 9999px;
      background: currentColor;
    }
    .btn-push {
      background: #4f46e5;
      color: white;
      border: none;
      padding: 0.375rem 0.75rem;
      border-radius: 0.5rem;
      font-size: 0.75rem;
      font-weight: 600;
      cursor: pointer;
    }
    .btn-logout {
      background: transparent;
      border: 1px solid #475569;
      color: #cbd5e1;
      padding: 0.375rem 0.75rem;
      border-radius: 0.5rem;
      font-size: 0.75rem;
      cursor: pointer;
    }
    .main-content {
      max-width: 1100px;
      margin: 0 auto;
      padding: 1.5rem;
    }
    .requests-section {
      background: rgba(245, 158, 11, 0.1);
      border: 1px solid rgba(245, 158, 11, 0.3);
      border-radius: 1rem;
      padding: 1.25rem;
      margin-bottom: 2rem;
    }
    .text-amber {
      color: #f59e0b;
    }
    .section-title {
      font-size: 1.125rem;
      font-weight: 700;
      margin: 0 0 0.25rem 0;
    }
    .section-subtitle {
      font-size: 0.8125rem;
      color: #94a3b8;
      margin: 0;
    }
    .requests-grid {
      display: grid;
      gap: 1rem;
      margin-top: 1rem;
      grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    }
    .request-card {
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 0.75rem;
      padding: 1rem;
    }
    .request-header {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
    }
    .request-child-name {
      font-size: 1.125rem;
      font-weight: 700;
      color: #f8fafc;
    }
    .request-device-name {
      font-size: 0.75rem;
      color: #94a3b8;
    }
    .badge-pending {
      background: #f59e0b;
      color: #000;
      font-size: 0.6875rem;
      font-weight: 700;
      padding: 0.2rem 0.5rem;
      border-radius: 0.375rem;
      text-transform: uppercase;
    }
    .request-msg {
      font-size: 0.875rem;
      color: #cbd5e1;
      margin: 0.75rem 0;
    }
    .grant-actions {
      display: flex;
      flex-wrap: wrap;
      gap: 0.375rem;
    }
    .btn-grant {
      background: #334155;
      color: #f8fafc;
      border: none;
      padding: 0.4rem 0.625rem;
      border-radius: 0.375rem;
      font-size: 0.8125rem;
      font-weight: 600;
      cursor: pointer;
      transition: background 0.15s;
    }
    .btn-grant:hover {
      background: #475569;
    }
    .btn-grant.primary {
      background: #10b981;
      color: #0f172a;
    }
    .btn-deny {
      background: rgba(239, 68, 68, 0.2);
      color: #fca5a5;
      border: 1px solid rgba(239, 68, 68, 0.3);
      padding: 0.4rem 0.625rem;
      border-radius: 0.375rem;
      font-size: 0.8125rem;
      font-weight: 600;
      cursor: pointer;
    }
    .section-header {
      display: flex;
      justify-content: space-between;
      align-items: flex-end;
      margin-bottom: 1.25rem;
    }
    .btn-primary-sm {
      background: #4f46e5;
      color: white;
      border: none;
      padding: 0.5rem 1rem;
      border-radius: 0.5rem;
      font-size: 0.8125rem;
      font-weight: 600;
      cursor: pointer;
    }
    .children-grid {
      display: grid;
      gap: 1.25rem;
      grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    }
    .child-card {
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 1rem;
      padding: 1.25rem;
    }
    .child-header {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      margin-bottom: 1rem;
    }
    .child-name {
      font-size: 1.25rem;
      font-weight: 700;
      margin: 0;
    }
    .emergency-tag {
      font-size: 0.75rem;
      color: #94a3b8;
      margin-top: 0.2rem;
    }
    .child-menu-btns {
      display: flex;
      gap: 0.375rem;
    }
    .btn-icon {
      background: #334155;
      border: none;
      color: #cbd5e1;
      padding: 0.375rem 0.625rem;
      border-radius: 0.375rem;
      font-size: 0.75rem;
      font-weight: 600;
      cursor: pointer;
    }
    .btn-icon:hover {
      background: #475569;
    }
    .devices-list {
      background: #0f172a;
      border-radius: 0.625rem;
      padding: 0.75rem;
      margin-bottom: 0.75rem;
    }
    .devices-label {
      font-size: 0.6875rem;
      text-transform: uppercase;
      font-weight: 700;
      color: #64748b;
      margin-bottom: 0.5rem;
    }
    .no-devices {
      font-size: 0.75rem;
      color: #94a3b8;
    }
    .device-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 0.375rem 0;
      border-bottom: 1px solid #1e293b;
    }
    .device-row:last-child {
      border-bottom: none;
    }
    .device-info {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    .device-name {
      font-size: 0.8125rem;
      font-weight: 600;
    }
    .device-platform {
      font-size: 0.6875rem;
      color: #94a3b8;
      margin-left: 0.25rem;
    }
    .device-badge {
      font-size: 0.6875rem;
      font-weight: 700;
      padding: 0.15rem 0.5rem;
      border-radius: 0.375rem;
      text-transform: uppercase;
    }
    .device-badge.active {
      background: rgba(34, 197, 94, 0.2);
      color: #86efac;
    }
    .device-badge.locked {
      background: rgba(245, 158, 11, 0.2);
      color: #fcd34d;
    }
    .device-badge.offline {
      background: rgba(148, 163, 184, 0.2);
      color: #cbd5e1;
    }
    .schedule-summary {
      display: flex;
      align-items: center;
      gap: 0.375rem;
      font-size: 0.75rem;
      color: #94a3b8;
      padding-top: 0.5rem;
      border-top: 1px solid #334155;
    }
    /* Modals */
    .modal-backdrop {
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.7);
      backdrop-filter: blur(4px);
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1rem;
      z-index: 50;
    }
    .modal-card {
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 1rem;
      padding: 1.5rem;
      width: 100%;
      max-width: 440px;
    }
    .modal-card.wide {
      max-width: 620px;
    }
    .modal-card.text-center {
      text-align: center;
    }
    .modal-subtitle {
      font-size: 0.8125rem;
      color: #94a3b8;
      margin-top: 0.25rem;
      margin-bottom: 1.25rem;
    }
    .pairing-code-box {
      font-family: monospace;
      font-size: 2.5rem;
      font-weight: 800;
      letter-spacing: 0.35rem;
      color: #818cf8;
      background: #0f172a;
      padding: 1rem;
      border-radius: 0.75rem;
      border: 2px dashed #4f46e5;
      margin: 1rem 0 0.5rem 0;
    }
    .code-timer {
      font-size: 0.75rem;
      color: #94a3b8;
    }
    .pairing-instructions {
      text-align: left;
      font-size: 0.8125rem;
      color: #cbd5e1;
      margin: 1.25rem 0;
      padding-left: 1.25rem;
    }
    .pairing-instructions li {
      margin-bottom: 0.375rem;
    }
    .form-group {
      margin-bottom: 1rem;
    }
    .form-group label {
      display: block;
      font-size: 0.75rem;
      color: #cbd5e1;
      margin-bottom: 0.25rem;
    }
    .form-group input {
      width: 100%;
      padding: 0.625rem 0.875rem;
      background: #0f172a;
      border: 1px solid #334155;
      border-radius: 0.5rem;
      color: #f8fafc;
      box-sizing: border-box;
    }
    .modal-actions {
      display: flex;
      justify-content: flex-end;
      gap: 0.5rem;
      margin-top: 1.25rem;
    }
    .modal-actions.center {
      justify-content: center;
    }
    .modal-header-between {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
    }
    .btn-close {
      background: transparent;
      border: none;
      color: #94a3b8;
      font-size: 1.25rem;
      cursor: pointer;
    }
    .btn-secondary {
      background: #334155;
      color: #cbd5e1;
      border: none;
      padding: 0.5rem 1rem;
      border-radius: 0.5rem;
      font-size: 0.8125rem;
      cursor: pointer;
    }
    .btn-primary {
      background: #4f46e5;
      color: white;
      border: none;
      padding: 0.5rem 1rem;
      border-radius: 0.5rem;
      font-size: 0.8125rem;
      font-weight: 600;
      cursor: pointer;
    }
    /* Schedule matrix */
    .schedule-matrix {
      display: flex;
      flex-direction: column;
      gap: 0.5rem;
      margin: 1rem 0;
      max-height: 55vh;
      overflow-y: auto;
    }
    .schedule-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      background: #0f172a;
      border: 1px solid #334155;
      border-radius: 0.5rem;
      padding: 0.625rem 0.875rem;
    }
    .schedule-row.enabled {
      border-color: #4f46e5;
    }
    .day-col {
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }
    .day-name {
      font-size: 0.875rem;
      font-weight: 600;
    }
    .time-col {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    .time-col.disabled {
      opacity: 0.35;
    }
    .time-col label {
      font-size: 0.75rem;
      color: #94a3b8;
    }
    .time-col input[type="time"] {
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 0.375rem;
      color: #f8fafc;
      padding: 0.25rem 0.5rem;
      font-size: 0.8125rem;
    }
    /* Simple switch */
    .toggle-switch {
      position: relative;
      display: inline-block;
      width: 34px;
      height: 20px;
    }
    .toggle-switch input {
      opacity: 0;
      width: 0;
      height: 0;
    }
    .slider {
      position: absolute;
      cursor: pointer;
      inset: 0;
      background-color: #334155;
      transition: .2s;
      border-radius: 20px;
    }
    .slider:before {
      position: absolute;
      content: "";
      height: 14px;
      width: 14px;
      left: 3px;
      bottom: 3px;
      background-color: white;
      transition: .2s;
      border-radius: 50%;
    }
    input:checked + .slider {
      background-color: #4f46e5;
    }
    input:checked + .slider:before {
      transform: translateX(14px);
    }
  `]
})
export class DashboardComponent implements OnInit, OnDestroy {
  api = inject(ApiService);
  auth = inject(AuthService);
  push = inject(PushService);
  realtime = inject(RealtimeService);

  loading = signal<boolean>(true);
  children = signal<ChildProfile[]>([]);
  pendingRequests = signal<ScreenTimeRequest[]>([]);

  // Add child modal state
  showAddChildModal = signal<boolean>(false);
  newChildName = '';
  newChildEmergency = '';

  // Pairing code modal state
  activePairingChild = signal<ChildProfile | null>(null);
  currentPairingCode = signal<string>('');

  // Weekly schedule modal state
  activeScheduleChild = signal<ChildProfile | null>(null);
  daysOfWeek = [
    { index: 0, name: 'Sunday' },
    { index: 1, name: 'Monday' },
    { index: 2, name: 'Tuesday' },
    { index: 3, name: 'Wednesday' },
    { index: 4, name: 'Thursday' },
    { index: 5, name: 'Friday' },
    { index: 6, name: 'Saturday' }
  ];
  scheduleState: { [day: number]: { enabled: boolean; start_time: string; end_time: string; id?: string } } = {};

  private sub?: Subscription;

  ngOnInit() {
    this.initScheduleState();
    this.refreshAll();

    // Listen to real-time events
    this.sub = this.realtime.events$.subscribe((event) => {
      if (event.type === 'SCREEN_TIME_REQUESTED') {
        this.refreshPendingRequests();
      } else if (event.type === 'REQUEST_RESOLVED') {
        this.refreshPendingRequests();
        this.refreshChildren();
      }
    });

    // Also poll every 10s as a fallback
    setInterval(() => {
      if (this.auth.isLoggedIn()) {
        this.refreshPendingRequests();
      }
    }, 10000);
  }

  ngOnDestroy() {
    this.sub?.unsubscribe();
  }

  private initScheduleState() {
    for (let i = 0; i <= 6; i++) {
      this.scheduleState[i] = {
        enabled: false,
        start_time: '15:00',
        end_time: '18:00'
      };
    }
  }

  refreshAll() {
    this.loading.set(true);
    this.refreshPendingRequests();
    this.refreshChildren();
  }

  refreshPendingRequests() {
    this.api.getPendingRequests().subscribe({
      next: (reqs) => this.pendingRequests.set(reqs),
      error: (e) => console.error('Failed to get requests:', e)
    });
  }

  refreshChildren() {
    this.api.getChildren().subscribe({
      next: (data) => {
        this.children.set(data);
        this.loading.set(false);
      },
      error: () => this.loading.set(false)
    });
  }

  resolve(requestId: string, action: 'GRANT' | 'DENY', minutes?: number) {
    this.api.resolveRequest(requestId, action, minutes).subscribe({
      next: () => {
        this.pendingRequests.update((list) => list.filter((r) => r.id !== requestId));
        this.refreshChildren();
      }
    });
  }

  createChild() {
    if (!this.newChildName.trim()) return;
    this.api.createChild({
      display_name: this.newChildName,
      emergency_contact: this.newChildEmergency
    }).subscribe({
      next: () => {
        this.showAddChildModal.set(false);
        this.newChildName = '';
        this.newChildEmergency = '';
        this.refreshChildren();
      }
    });
  }

  openPairingModal(child: ChildProfile) {
    this.activePairingChild.set(child);
    this.currentPairingCode.set('');
    this.api.generatePairingCode(child.id).subscribe({
      next: (res) => this.currentPairingCode.set(res.pairing_code)
    });
  }

  openScheduleModal(child: ChildProfile) {
    this.activeScheduleChild.set(child);
    this.initScheduleState();

    if (child.schedules) {
      for (const rule of child.schedules) {
        this.scheduleState[rule.day_of_week] = {
          enabled: rule.enabled,
          start_time: rule.start_time,
          end_time: rule.end_time,
          id: rule.id
        };
      }
    }
  }

  saveScheduleRules() {
    const child = this.activeScheduleChild();
    if (!child) return;

    const rules: ScheduleRule[] = [];
    for (let day = 0; day <= 6; day++) {
      const state = this.scheduleState[day];
      rules.push({
        id: state.id,
        child_id: child.id,
        day_of_week: day,
        start_time: state.start_time || '15:00',
        end_time: state.end_time || '18:00',
        timezone: 'Local',
        enabled: state.enabled
      });
    }

    this.api.saveBatchSchedule(child.id, rules).subscribe({
      next: () => {
        this.activeScheduleChild.set(null);
        this.refreshChildren();
      },
      error: (err) => {
        alert('Failed to save schedule: ' + (err.error?.error || err.message));
      }
    });
  }

  getScheduleSummary(child: ChildProfile): string {
    if (!child.schedules || child.schedules.length === 0) {
      return 'No unrestricted schedule set (locked 24/7)';
    }
    const enabled = child.schedules.filter((s) => s.enabled);
    if (enabled.length === 0) {
      return 'No unrestricted schedule set (locked 24/7)';
    }
    return `${enabled.length} recurring day(s) configured`;
  }

  enablePush() {
    this.push.subscribe();
  }
}

