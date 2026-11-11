import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Observable } from 'rxjs';
import {
  AuthResponse,
  ChildProfile,
  FamilyUser,
  PairingCodeResponse,
  ScheduleRule,
  ScreenTimeRequest
} from '../models/types';

@Injectable({
  providedIn: 'root'
})
export class ApiService {
  private http = inject(HttpClient);
  private baseUrl = ''; // Relative path so proxy or same-origin works; fallback to localhost:8080 if standalone dev

  constructor() {
    if (typeof window !== 'undefined' && window.location.port === '4200') {
      this.baseUrl = 'http://localhost:8080';
    }
  }

  private getHeaders(): HttpHeaders {
    const token = localStorage.getItem('screentime_token') || '';
    return new HttpHeaders({
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`
    });
  }

  // Auth
  register(data: { family_name: string; display_name: string; email: string; password: string }): Observable<AuthResponse> {
    return this.http.post<AuthResponse>(`${this.baseUrl}/api/auth/register`, data);
  }

  login(data: { email: string; password: string }): Observable<AuthResponse> {
    return this.http.post<AuthResponse>(`${this.baseUrl}/api/auth/login`, data);
  }

  getMe(): Observable<{ user: FamilyUser; family_name: string }> {
    return this.http.get<{ user: FamilyUser; family_name: string }>(`${this.baseUrl}/api/auth/me`, {
      headers: this.getHeaders()
    });
  }

  // Family Members
  getFamilyMembers(): Observable<FamilyUser[]> {
    return this.http.get<FamilyUser[]>(`${this.baseUrl}/api/family/members`, {
      headers: this.getHeaders()
    });
  }

  addFamilyMember(data: { email: string; display_name: string; password: string }): Observable<{ id: string }> {
    return this.http.post<{ id: string }>(`${this.baseUrl}/api/family/members`, data, {
      headers: this.getHeaders()
    });
  }

  // Children
  getChildren(): Observable<ChildProfile[]> {
    return this.http.get<ChildProfile[]>(`${this.baseUrl}/api/children`, {
      headers: this.getHeaders()
    });
  }

  createChild(data: { display_name: string; emergency_contact: string }): Observable<{ id: string }> {
    return this.http.post<{ id: string }>(`${this.baseUrl}/api/children`, data, {
      headers: this.getHeaders()
    });
  }

  deleteChild(id: string): Observable<any> {
    return this.http.delete(`${this.baseUrl}/api/children/${id}`, {
      headers: this.getHeaders()
    });
  }

  // Pairing
  generatePairingCode(childId: string): Observable<PairingCodeResponse> {
    return this.http.post<PairingCodeResponse>(
      `${this.baseUrl}/api/children/${childId}/pairing-code`,
      {},
      { headers: this.getHeaders() }
    );
  }

  // Schedules
  getChildSchedules(childId: string): Observable<ScheduleRule[]> {
    return this.http.get<ScheduleRule[]>(`${this.baseUrl}/api/children/${childId}/schedules`, {
      headers: this.getHeaders()
    });
  }

  saveSchedule(childId: string, rule: ScheduleRule): Observable<{ id: string }> {
    return this.http.post<{ id: string }>(`${this.baseUrl}/api/children/${childId}/schedules`, rule, {
      headers: this.getHeaders()
    });
  }

  deleteSchedule(id: string): Observable<any> {
    return this.http.delete(`${this.baseUrl}/api/schedules/${id}`, {
      headers: this.getHeaders()
    });
  }

  // Requests
  getPendingRequests(): Observable<ScreenTimeRequest[]> {
    return this.http.get<ScreenTimeRequest[]>(`${this.baseUrl}/api/requests/pending`, {
      headers: this.getHeaders()
    });
  }

  resolveRequest(requestId: string, action: 'GRANT' | 'DENY', grantMinutes?: number): Observable<any> {
    return this.http.post(
      `${this.baseUrl}/api/requests/${requestId}/resolve`,
      { action, grant_minutes: grantMinutes },
      { headers: this.getHeaders() }
    );
  }

  // Push
  getVapidPublicKey(): Observable<{ public_key: string }> {
    return this.http.get<{ public_key: string }>(`${this.baseUrl}/api/push/vapid-public-key`);
  }

  subscribePush(sub: any): Observable<any> {
    return this.http.post(`${this.baseUrl}/api/push/subscribe`, sub, {
      headers: this.getHeaders()
    });
  }
}
