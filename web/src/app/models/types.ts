export interface FamilyUser {
  id: string;
  family_id: string;
  email: string;
  display_name: string;
  created_at: string;
}

export interface ChildProfile {
  id: string;
  family_id: string;
  display_name: string;
  emergency_contact: string;
  created_at: string;
  devices?: Device[];
  schedules?: ScheduleRule[];
}

export interface Device {
  id: string;
  child_id: string;
  platform: 'WINDOWS' | 'ANDROID';
  device_name: string;
  pairing_code?: string;
  last_seen_at?: string;
  status: 'LOCKED' | 'ACTIVE' | 'TAMPERED' | 'OFFLINE' | 'PENDING';
}

export interface ScheduleRule {
  id?: string;
  child_id?: string;
  day_of_week: number; // 0 = Sunday, 1 = Monday, ..., 6 = Saturday
  start_time: string;  // "15:00"
  end_time: string;    // "18:00"
  timezone?: string;
  enabled: boolean;
}

export interface ScreenTimeRequest {
  id: string;
  device_id: string;
  child_id: string;
  child_display_name?: string;
  device_name?: string;
  status: 'PENDING' | 'APPROVED' | 'DENIED' | 'EXPIRED';
  created_at: string;
}

export interface AuthResponse {
  token: string;
  user: FamilyUser;
}

export interface PairingCodeResponse {
  pairing_code: string;
  expires_at: string;
}
