import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { Subject, of } from 'rxjs';
import { DashboardComponent } from './dashboard.component';
import { ApiService } from '../../services/api.service';
import { AuthService } from '../../services/auth.service';
import { PushService } from '../../services/push.service';
import { PwaInstallService } from '../../services/pwa-install.service';
import { RealtimeService } from '../../services/realtime.service';
import { ChildProfile } from '../../models/types';

describe('DashboardComponent', () => {
  let fixture: ComponentFixture<DashboardComponent>;
  let mockChildren: ChildProfile[];
  let setDeviceGrantSpy: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    const futureIso = new Date(Date.now() + 60 * 60 * 1000).toISOString();
    mockChildren = [
      {
        id: 'child-1',
        family_id: 'fam-1',
        display_name: 'Lucas',
        emergency_contact: '555-0199',
        created_at: new Date().toISOString(),
        devices: [
          {
            id: 'dev-active',
            child_id: 'child-1',
            platform: 'WINDOWS',
            device_name: 'Lucas PC',
            status: 'ACTIVE',
            valid_until: futureIso,
            remaining_seconds: 3600,
            access_reason: 'GRANT'
          },
          {
            id: 'dev-locked',
            child_id: 'child-1',
            platform: 'ANDROID',
            device_name: 'Lucas Phone',
            status: 'LOCKED'
          }
        ],
        schedules: []
      }
    ];

    setDeviceGrantSpy = vi.fn().mockReturnValue(of({ status: 'ok' }));

    const apiMock = {
      getPendingRequests: vi.fn().mockReturnValue(of([])),
      getChildren: vi.fn().mockReturnValue(of(mockChildren)),
      setDeviceGrant: setDeviceGrantSpy,
      getWindowsInstallerUrl: vi.fn().mockReturnValue('/downloads/ScreenTimeSetup.exe')
    };

    const authMock = {
      familyName: signal('The Millers'),
      currentUser: signal({
        id: 'user-1',
        family_id: 'fam-1',
        email: 'dad@example.com',
        display_name: 'Dad',
        created_at: ''
      }),
      isLoggedIn: vi.fn().mockReturnValue(true),
      logout: vi.fn()
    };

    const pushMock = {
      isSupported: signal(false),
      isSubscribed: signal(false),
      checkSubscription: vi.fn(),
      subscribe: vi.fn()
    };

    const pwaMock = {
      canInstall: signal(false),
      install: vi.fn()
    };

    const realtimeMock = {
      isConnected: signal(true),
      events$: new Subject<any>()
    };

    await TestBed.configureTestingModule({
      imports: [DashboardComponent],
      providers: [
        { provide: ApiService, useValue: apiMock },
        { provide: AuthService, useValue: authMock },
        { provide: PushService, useValue: pushMock },
        { provide: PwaInstallService, useValue: pwaMock },
        { provide: RealtimeService, useValue: realtimeMock }
      ]
    }).compileComponents();

    fixture = TestBed.createComponent(DashboardComponent);
    fixture.detectChanges();
  });

  afterEach(() => {
    fixture.destroy();
  });

  it('renders "🔒 Lock device" instead of "Lock Now" on device rows', () => {
    const el = fixture.nativeElement as HTMLElement;
    const lockButtons = Array.from(el.querySelectorAll('.btn-device-lock')) as HTMLButtonElement[];

    expect(lockButtons.length).toBe(2);
    for (const btn of lockButtons) {
      expect(btn.textContent?.trim()).toBe('🔒 Lock device');
    }
    expect(el.textContent).not.toContain('Lock Now');
  });

  it('opens the confirmation dialog with "Lock now" selected by default and locks immediately on confirm', () => {
    const el = fixture.nativeElement as HTMLElement;
    const firstLockBtn = el.querySelector('.btn-device-lock') as HTMLButtonElement;
    firstLockBtn.click();
    fixture.detectChanges();

    const radios = Array.from(
      el.querySelectorAll('input[type="radio"][name="lockTimingOption"]')
    ) as HTMLInputElement[];
    expect(radios.length).toBe(2);

    const nowRadio = radios.find((r) => r.value === 'NOW')!;
    const fiveMinRadio = radios.find((r) => r.value === 'FIVE_MINUTES')!;
    expect(nowRadio).toBeTruthy();
    expect(fiveMinRadio).toBeTruthy();
    expect(nowRadio.checked).toBe(true);
    expect(fiveMinRadio.checked).toBe(false);

    const confirmBtn = el.querySelector('.btn-modal-confirm') as HTMLButtonElement;
    expect(confirmBtn.textContent?.trim()).toBe('🔒 Lock device');
    confirmBtn.click();
    fixture.detectChanges();

    expect(setDeviceGrantSpy).toHaveBeenCalledTimes(1);
    expect(setDeviceGrantSpy).toHaveBeenCalledWith('dev-active', 'LOCK');
    expect(el.querySelector('input[type="radio"][name="lockTimingOption"]')).toBeNull();
  });

  it('sends grant_minutes: 5 when "Lock in 5 minutes" is selected and confirmed', () => {
    const el = fixture.nativeElement as HTMLElement;
    const firstLockBtn = el.querySelector('.btn-device-lock') as HTMLButtonElement;
    firstLockBtn.click();
    fixture.detectChanges();

    const fiveMinRadio = el.querySelector(
      'input[type="radio"][name="lockTimingOption"][value="FIVE_MINUTES"]'
    ) as HTMLInputElement;
    fiveMinRadio.checked = true;
    fiveMinRadio.dispatchEvent(new Event('change'));
    fixture.detectChanges();

    const confirmBtn = el.querySelector('.btn-modal-confirm') as HTMLButtonElement;
    confirmBtn.click();
    fixture.detectChanges();

    expect(setDeviceGrantSpy).toHaveBeenCalledTimes(1);
    expect(setDeviceGrantSpy).toHaveBeenCalledWith('dev-active', 'SET', undefined, 5);
    expect(el.querySelector('input[type="radio"][name="lockTimingOption"]')).toBeNull();
  });

  it('resets the default option to "Lock now" when cancelled and reopened, and opens from Edit Time modal', () => {
    const el = fixture.nativeElement as HTMLElement;
    const firstLockBtn = el.querySelector('.btn-device-lock') as HTMLButtonElement;
    firstLockBtn.click();
    fixture.detectChanges();

    const fiveMinRadio = el.querySelector(
      'input[type="radio"][name="lockTimingOption"][value="FIVE_MINUTES"]'
    ) as HTMLInputElement;
    fiveMinRadio.checked = true;
    fiveMinRadio.dispatchEvent(new Event('change'));
    fixture.detectChanges();

    const cancelBtn = Array.from(el.querySelectorAll('.modal-actions button')).find(
      (b) => b.textContent?.trim() === 'Cancel'
    ) as HTMLButtonElement;
    cancelBtn.click();
    fixture.detectChanges();

    const editBtn = el.querySelector('.btn-device-edit') as HTMLButtonElement;
    editBtn.click();
    fixture.detectChanges();

    const modalLockBtn = el.querySelector('.modal-actions-split .btn-deny') as HTMLButtonElement;
    expect(modalLockBtn.textContent?.trim()).toBe('🔒 Lock device');
    modalLockBtn.click();
    fixture.detectChanges();

    const nowRadio = el.querySelector(
      'input[type="radio"][name="lockTimingOption"][value="NOW"]'
    ) as HTMLInputElement;
    expect(nowRadio.checked).toBe(true);
    expect(setDeviceGrantSpy).not.toHaveBeenCalled();
  });
});
