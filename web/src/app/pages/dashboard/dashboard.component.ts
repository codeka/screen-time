import { Component, OnInit, OnDestroy, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ApiService } from '../../services/api.service';
import { AuthService } from '../../services/auth.service';
import { PushService } from '../../services/push.service';
import { RealtimeService } from '../../services/realtime.service';
import { ChildProfile, ScheduleRule, ScreenTimeRequest, Device } from '../../models/types';
import { Subscription } from 'rxjs';

interface EndTimeModalTarget {
  mode: 'REQUEST' | 'EDIT_ACTIVE' | 'UNLOCK_DEVICE';
  requestId?: string;
  deviceId: string;
  childName: string;
  deviceName: string;
  currentValidUntil?: string;
}

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
                    <button class="btn-grant custom" (click)="openCustomTimeForRequest(req)">🕐 Custom…</button>
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
                              <div>
                                <span class="device-name">{{ dev.device_name }}</span>
                                <span class="device-platform">({{ dev.platform }})</span>
                              </div>
                              @if (getRemainingSeconds(dev) > 0 && dev.valid_until) {
                                <div class="device-expiry">
                                  ⏱️ Locks at <strong>{{ formatClockTime(dev.valid_until) }}</strong>
                                  <span class="expiry-countdown">({{ formatRemaining(getRemainingSeconds(dev)) }} left)</span>
                                  @if (dev.access_reason === 'SCHEDULE') {
                                    <span class="expiry-reason">· Schedule</span>
                                  }
                                </div>
                              }
                            </div>
                          </div>

                          <div class="device-right-actions">
                            @if (getRemainingSeconds(dev) > 0) {
                              <span class="device-badge active">ACTIVE</span>
                              <button class="btn-device-edit" (click)="openEditEndTimeForDevice(child, dev)" title="Change when this device locks">
                                ✏️ Edit Time
                              </button>
                              <button
                                class="btn-device-lock"
                                [disabled]="lockingDeviceId() === dev.id"
                                (click)="forceLockDevice(dev)"
                                title="Force this device to lock immediately">
                                {{ lockingDeviceId() === dev.id ? 'Locking…' : '🔒 Lock Now' }}
                              </button>
                            } @else {
                              <span class="device-badge" [ngClass]="dev.status === 'OFFLINE' ? 'offline' : 'locked'">
                                {{ dev.status === 'OFFLINE' ? 'OFFLINE' : 'LOCKED' }}
                              </span>
                              <button class="btn-device-unlock" (click)="openEditEndTimeForDevice(child, dev)" title="Unlock until a specific time">
                                🔓 Unlock…
                              </button>
                              <button
                                class="btn-device-lock"
                                [disabled]="lockingDeviceId() === dev.id"
                                (click)="forceLockDevice(dev)"
                                title="Force this device to lock immediately">
                                {{ lockingDeviceId() === dev.id ? 'Locking…' : '🔒 Lock Now' }}
                              </button>
                            }
                          </div>
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

      <!-- Set / Edit End Time Modal -->
      @if (endTimeModal(); as target) {
        <div class="modal-backdrop" (click)="endTimeModal.set(null)">
          <div class="modal-card" (click)="$event.stopPropagation()">
            <div class="modal-header-between">
              <div>
                <h3>
                  @if (target.mode === 'EDIT_ACTIVE') {
                    Edit End Time
                  } @else if (target.mode === 'REQUEST') {
                    Custom Expiry Time
                  } @else {
                    Unlock Until Time
                  }
                </h3>
                <p class="modal-subtitle">
                  <strong>{{ target.childName }}</strong> · {{ target.deviceName }}
                  @if (target.currentValidUntil) {
                    <br />
                    <span class="current-expiry-note">
                      Currently locks at {{ formatClockTime(target.currentValidUntil) }}
                    </span>
                  }
                </p>
              </div>
              <button class="btn-close" (click)="endTimeModal.set(null)">✕</button>
            </div>

            <div class="form-group">
              <label>Lock Device At</label>
              <input
                type="time"
                class="time-picker-lg"
                [(ngModel)]="endTimeInput" />
            </div>

            <!-- Quick preset chips -->
            <div class="preset-section">
              <div class="preset-label">Quick Presets:</div>
              <div class="preset-chips">
                <button type="button" class="chip" (click)="applyRelativeMinutes(15)">+15m</button>
                <button type="button" class="chip" (click)="applyRelativeMinutes(30)">+30m</button>
                <button type="button" class="chip" (click)="applyRelativeMinutes(60)">+1h</button>
                <button type="button" class="chip" (click)="applyRelativeMinutes(120)">+2h</button>
                @for (preset of upcomingHourPresets(); track preset.value) {
                  <button
                    type="button"
                    class="chip"
                    [class.selected]="endTimeInput === preset.value"
                    (click)="endTimeInput = preset.value">
                    {{ preset.label }}
                  </button>
                }
              </div>
            </div>

            @if (getTargetPreview(); as preview) {
              <div class="expiry-preview-banner">
                🔒 Device will lock <strong>{{ preview.dayLabel }} at {{ preview.clockLabel }}</strong>
                <span class="preview-duration">({{ preview.durationLabel }} from now)</span>
              </div>
            }

            @if (endTimeError()) {
              <div class="inline-error">{{ endTimeError() }}</div>
            }

            <div class="modal-actions-split">
              <div>
                @if (target.mode === 'EDIT_ACTIVE') {
                  <button
                    type="button"
                    class="btn-deny"
                    [disabled]="savingEndTime()"
                    (click)="lockDeviceNow()">
                    🔒 Lock Now
                  </button>
                }
              </div>
              <div class="modal-actions-right">
                <button type="button" class="btn-secondary" (click)="endTimeModal.set(null)">Cancel</button>
                <button
                  type="button"
                  class="btn-primary"
                  [disabled]="savingEndTime() || !endTimeInput"
                  (click)="submitEndTime()">
                  @if (savingEndTime()) {
                    Saving...
                  } @else if (target.mode === 'EDIT_ACTIVE') {
                    Save End Time
                  } @else {
                    Unlock Until {{ getTargetPreview()?.clockLabel || endTimeInput }}
                  }
                </button>
              </div>
            </div>
          </div>
        </div>
      }

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
    .btn-grant.custom {
      background: rgba(99, 102, 241, 0.25);
      color: #c7d2fe;
      border: 1px solid rgba(99, 102, 241, 0.45);
    }
    .btn-grant.custom:hover {
      background: rgba(99, 102, 241, 0.4);
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
    .btn-deny:hover:not(:disabled) {
      background: rgba(239, 68, 68, 0.32);
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
      grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
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
      gap: 0.5rem;
      padding: 0.5rem 0;
      border-bottom: 1px solid #1e293b;
    }
    .device-row:last-child {
      border-bottom: none;
    }
    .device-info {
      display: flex;
      align-items: flex-start;
      gap: 0.5rem;
      min-width: 0;
    }
    .platform-icon {
      margin-top: 0.1rem;
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
    .device-expiry {
      margin-top: 0.2rem;
      font-size: 0.75rem;
      color: #86efac;
    }
    .device-expiry strong {
      color: #bbf7d0;
    }
    .expiry-countdown {
      color: #94a3b8;
      margin-left: 0.25rem;
    }
    .expiry-reason {
      color: #818cf8;
      margin-left: 0.25rem;
    }
    .device-right-actions {
      display: flex;
      align-items: center;
      gap: 0.4rem;
      flex-shrink: 0;
    }
    .btn-device-edit {
      background: rgba(99, 102, 241, 0.2);
      border: 1px solid rgba(99, 102, 241, 0.4);
      color: #c7d2fe;
      font-size: 0.6875rem;
      font-weight: 600;
      padding: 0.2rem 0.5rem;
      border-radius: 0.375rem;
      cursor: pointer;
      transition: background 0.15s;
    }
    .btn-device-edit:hover {
      background: rgba(99, 102, 241, 0.35);
    }
    .btn-device-unlock {
      background: #1e293b;
      border: 1px solid #334155;
      color: #cbd5e1;
      font-size: 0.6875rem;
      font-weight: 600;
      padding: 0.2rem 0.5rem;
      border-radius: 0.375rem;
      cursor: pointer;
      transition: background 0.15s;
    }
    .btn-device-unlock:hover {
      background: #334155;
    }
    .btn-device-lock {
      background: rgba(239, 68, 68, 0.18);
      border: 1px solid rgba(239, 68, 68, 0.38);
      color: #fca5a5;
      font-size: 0.6875rem;
      font-weight: 600;
      padding: 0.2rem 0.5rem;
      border-radius: 0.375rem;
      cursor: pointer;
      transition: background 0.15s;
    }
    .btn-device-lock:hover:not(:disabled) {
      background: rgba(239, 68, 68, 0.32);
    }
    .btn-device-lock:disabled {
      opacity: 0.6;
      cursor: not-allowed;
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
    .modal-card h3 {
      margin: 0;
    }
    .modal-subtitle {
      font-size: 0.8125rem;
      color: #94a3b8;
      margin-top: 0.25rem;
      margin-bottom: 1.25rem;
    }
    .current-expiry-note {
      color: #86efac;
      font-size: 0.75rem;
    }
    .time-picker-lg {
      width: 100%;
      font-size: 1.5rem !important;
      font-weight: 700;
      padding: 0.75rem 1rem !important;
      text-align: center;
      color-scheme: dark;
    }
    .preset-section {
      margin-bottom: 1rem;
    }
    .preset-label {
      font-size: 0.75rem;
      color: #94a3b8;
      margin-bottom: 0.4rem;
    }
    .preset-chips {
      display: flex;
      flex-wrap: wrap;
      gap: 0.375rem;
    }
    .chip {
      background: #0f172a;
      border: 1px solid #334155;
      color: #cbd5e1;
      font-size: 0.75rem;
      font-weight: 600;
      padding: 0.3rem 0.6rem;
      border-radius: 9999px;
      cursor: pointer;
      transition: all 0.15s;
    }
    .chip:hover {
      border-color: #6366f1;
      color: #f8fafc;
    }
    .chip.selected {
      background: #4f46e5;
      border-color: #4f46e5;
      color: white;
    }
    .expiry-preview-banner {
      background: rgba(16, 185, 129, 0.12);
      border: 1px solid rgba(16, 185, 129, 0.3);
      color: #d1fae5;
      border-radius: 0.5rem;
      padding: 0.65rem 0.85rem;
      font-size: 0.8125rem;
      margin-bottom: 0.75rem;
    }
    .preview-duration {
      color: #6ee7b7;
      margin-left: 0.25rem;
    }
    .inline-error {
      background: rgba(239, 68, 68, 0.15);
      border: 1px solid rgba(239, 68, 68, 0.35);
      color: #fca5a5;
      border-radius: 0.5rem;
      padding: 0.5rem 0.75rem;
      font-size: 0.8125rem;
      margin-bottom: 0.75rem;
    }
    .modal-actions-split {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 0.5rem;
      margin-top: 1.25rem;
    }
    .modal-actions-right {
      display: flex;
      gap: 0.5rem;
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
    .btn-primary:disabled {
      opacity: 0.6;
      cursor: not-allowed;
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
  nowMs = signal<number>(Date.now());

  // Set / Edit End Time modal state
  endTimeModal = signal<EndTimeModalTarget | null>(null);
  endTimeInput = '';
  savingEndTime = signal<boolean>(false);
  lockingDeviceId = signal<string | null>(null);
  endTimeError = signal<string>('');
  upcomingHourPresets = signal<{ label: string; value: string }[]>([]);

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
  private pollInterval?: ReturnType<typeof setInterval>;
  private clockInterval?: ReturnType<typeof setInterval>;
  private pendingCustomRequestId: string | null = null;

  private readonly boundSwListener = (event: MessageEvent) => {
    const msg = event.data;
    if (!msg) return;
    if (msg.type === 'OPEN_CUSTOM_GRANT' && msg.requestId) {
      if (msg.deviceId) {
        this.openModalWithDefaults({
          mode: 'REQUEST',
          requestId: msg.requestId,
          deviceId: msg.deviceId,
          childName: msg.childName || 'Child',
          deviceName: msg.deviceName || 'Device'
        });
      } else {
        this.pendingCustomRequestId = msg.requestId;
      }
      this.refreshPendingRequests();
      this.refreshChildren();
    } else if (msg.type === 'REFRESH_DASHBOARD') {
      this.refreshPendingRequests();
      this.refreshChildren();
    }
  };

  ngOnInit() {
    this.initScheduleState();
    this.checkInitialCustomRequestParam();
    this.refreshAll();

    // Listen to real-time WebSocket events
    this.sub = this.realtime.events$.subscribe((event) => {
      if (event.type === 'SCREEN_TIME_REQUESTED') {
        this.refreshPendingRequests();
      } else if (event.type === 'REQUEST_RESOLVED' || event.type === 'DEVICE_GRANT_UPDATED') {
        this.refreshPendingRequests();
        this.refreshChildren();
      }
    });

      // Listen to Service Worker messages (e.g. clicking "Custom…" on a push notification)
      if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
        navigator.serviceWorker.addEventListener('message', this.boundSwListener);
      }

      // Live 1-second clock tick so remaining countdowns update smoothly
      this.clockInterval = setInterval(() => {
        this.nowMs.set(Date.now());
      }, 1000);

      // Poll every 10s as a fallback to keep requests and device expiry states synced
      this.pollInterval = setInterval(() => {
        if (this.auth.isLoggedIn()) {
          this.refreshPendingRequests();
          this.refreshChildren();
        }
      }, 10000);
    }

  ngOnDestroy() {
      this.sub?.unsubscribe();
      if(this.pollInterval) clearInterval(this.pollInterval);
    if (this.clockInterval) clearInterval(this.clockInterval);
    if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
      navigator.serviceWorker.removeEventListener('message', this.boundSwListener);
    }
  }

  private checkInitialCustomRequestParam() {
    if (typeof window === 'undefined') return;
    const params = new URLSearchParams(window.location.search);
    const fromUrl = params.get('custom_request');
    const fromSession = sessionStorage.getItem('screentime_custom_request');
    const reqId = fromUrl || fromSession;
    if (reqId) {
      this.pendingCustomRequestId = reqId;
      sessionStorage.removeItem('screentime_custom_request');
      if (fromUrl) {
        window.history.replaceState({}, '', window.location.pathname);
      }
    }
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
      next: (reqs) => {
        this.pendingRequests.set(reqs);
        if (this.pendingCustomRequestId) {
          const targetId = this.pendingCustomRequestId;
          this.pendingCustomRequestId = null;
          const match = reqs.find((r) => r.id === targetId);
          if (match) {
            this.openCustomTimeForRequest(match);
          }
        }
      },
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

  // --- End Time Display & Editing Helpers ---

  getRemainingSeconds(dev: Device): number {
    if (!dev.valid_until) return 0;
    const expiryMs = new Date(dev.valid_until).getTime();
    if (isNaN(expiryMs)) return 0;
    const diff = Math.floor((expiryMs - this.nowMs()) / 1000);
    return diff > 0 ? diff : 0;
  }

  formatClockTime(isoString: string): string {
    const dt = new Date(isoString);
    if (isNaN(dt.getTime())) return '';
    return dt.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }

  formatRemaining(seconds: number): string {
    if (seconds <= 0) return '0m';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.ceil((seconds % 3600) / 60);
    if (hrs > 0 && mins === 60) {
      return `${hrs + 1}h 0m`;
    }
    if (hrs > 0) {
      return `${hrs}h ${mins}m`;
    }
    if (seconds < 60) {
      return `${seconds}s`;
    }
    return `${mins}m`;
  }

  openCustomTimeForRequest(req: ScreenTimeRequest) {
    this.openModalWithDefaults({
      mode: 'REQUEST',
      requestId: req.id,
      deviceId: req.device_id,
      childName: req.child_display_name || 'Child',
      deviceName: req.device_name || 'Device'
    });
  }

  openEditEndTimeForDevice(child: ChildProfile, dev: Device) {
    const remaining = this.getRemainingSeconds(dev);
    this.openModalWithDefaults({
      mode: remaining > 0 ? 'EDIT_ACTIVE' : 'UNLOCK_DEVICE',
      deviceId: dev.id,
      childName: child.display_name,
      deviceName: dev.device_name,
      currentValidUntil: remaining > 0 ? dev.valid_until : undefined
    });
  }

  private openModalWithDefaults(target: EndTimeModalTarget) {
    this.endTimeError.set('');
    this.savingEndTime.set(false);
    this.buildUpcomingHourPresets();

    if (target.currentValidUntil) {
      const existing = new Date(target.currentValidUntil);
      this.endTimeInput = this.dateToHHMM(existing);
    } else {
      // Default to 1 hour from now rounded to the nearest 15 minutes
      const def = new Date(Date.now() + 60 * 60 * 1000);
      const roundedMins = Math.ceil(def.getMinutes() / 15) * 15;
      def.setMinutes(roundedMins, 0, 0);
      this.endTimeInput = this.dateToHHMM(def);
    }

    this.endTimeModal.set(target);
  }

  private buildUpcomingHourPresets() {
    const now = new Date();
    const presets: { label: string; value: string }[] = [];
    for (let i = 1; i <= 5; i++) {
      const d = new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours() + i, 0, 0, 0);
      presets.push({
        label: d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }),
        value: this.dateToHHMM(d)
      });
    }
    this.upcomingHourPresets.set(presets);
  }

  applyRelativeMinutes(mins: number) {
    const d = new Date(Date.now() + mins * 60 * 1000);
    this.endTimeInput = this.dateToHHMM(d);
  }

  private dateToHHMM(d: Date): string {
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    return `${hh}:${mm}`;
  }

  private computeTargetDate(): Date | null {
    if (!this.endTimeInput || !this.endTimeInput.includes(':')) return null;
    const [hStr, mStr] = this.endTimeInput.split(':');
    const hours = parseInt(hStr, 10);
    const mins = parseInt(mStr, 10);
    if (isNaN(hours) || isNaN(mins)) return null;

    const now = new Date(this.nowMs());
    const target = new Date(now.getFullYear(), now.getMonth(), now.getDate(), hours, mins, 0, 0);
    // If chosen HH:MM is at or earlier than current time, roll to tomorrow
    if (target.getTime() <= now.getTime()) {
      target.setDate(target.getDate() + 1);
    }
    return target;
  }

  getTargetPreview(): { dayLabel: string; clockLabel: string; durationLabel: string } | null {
    const target = this.computeTargetDate();
    if (!target) return null;
    const now = new Date(this.nowMs());
    const isToday = target.getDate() === now.getDate() && target.getMonth() === now.getMonth();
    const diffSecs = Math.max(1, Math.floor((target.getTime() - now.getTime()) / 1000));
    return {
      dayLabel: isToday ? 'today' : 'tomorrow',
      clockLabel: target.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }),
      durationLabel: this.formatRemaining(diffSecs)
    };
  }

  submitEndTime() {
    const targetModal = this.endTimeModal();
    const targetDate = this.computeTargetDate();
    if (!targetModal || !targetDate) return;

    this.savingEndTime.set(true);
    this.endTimeError.set('');
    const validUntilIso = targetDate.toISOString();

    if (targetModal.mode === 'REQUEST' && targetModal.requestId) {
      const reqId = targetModal.requestId;
      this.api.resolveRequest(reqId, 'GRANT', undefined, validUntilIso).subscribe({
        next: () => {
          this.savingEndTime.set(false);
          this.endTimeModal.set(null);
          this.pendingRequests.update((list) => list.filter((r) => r.id !== reqId));
          this.refreshChildren();
        },
        error: (err) => {
          // If the request was already resolved in the meantime, fall back to updating the device grant directly
          if (err.status === 404 && targetModal.deviceId) {
            this.api.setDeviceGrant(targetModal.deviceId, 'SET', validUntilIso).subscribe({
              next: () => {
                this.savingEndTime.set(false);
                this.endTimeModal.set(null);
                this.refreshPendingRequests();
                this.refreshChildren();
              },
              error: (e2) => {
                this.savingEndTime.set(false);
                this.endTimeError.set(e2.error?.error || 'Failed to set end time.');
              }
            });
            return;
          }
          this.savingEndTime.set(false);
          this.endTimeError.set(err.error?.error || 'Failed to approve request.');
        }
      });
    } else {
      this.api.setDeviceGrant(targetModal.deviceId, 'SET', validUntilIso).subscribe({
        next: () => {
          this.savingEndTime.set(false);
          this.endTimeModal.set(null);
          this.refreshPendingRequests();
          this.refreshChildren();
        },
        error: (err) => {
          this.savingEndTime.set(false);
          this.endTimeError.set(err.error?.error || 'Failed to update end time.');
        }
      });
    }
  }

  forceLockDevice(dev: Device) {
    this.lockingDeviceId.set(dev.id);
    this.api.setDeviceGrant(dev.id, 'LOCK').subscribe({
      next: () => {
        this.lockingDeviceId.set(null);
        this.refreshPendingRequests();
        this.refreshChildren();
      },
      error: (err) => {
        this.lockingDeviceId.set(null);
        alert('Failed to lock device: ' + (err.error?.error || err.message));
      }
    });
  }

  lockDeviceNow() {
    const targetModal = this.endTimeModal();
    if (!targetModal) return;

    this.savingEndTime.set(true);
    this.endTimeError.set('');
    this.api.setDeviceGrant(targetModal.deviceId, 'LOCK').subscribe({
      next: () => {
        this.savingEndTime.set(false);
        this.endTimeModal.set(null);
        this.refreshPendingRequests();
        this.refreshChildren();
      },
      error: (err) => {
        this.savingEndTime.set(false);
        this.endTimeError.set(err.error?.error || 'Failed to lock device.');
      }
    });
  }

  // --- Child, Pairing, and Schedule Management ---

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
