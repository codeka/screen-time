package models

import "time"

type Family struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type FamilyUser struct {
	ID           string    `json:"id"`
	FamilyID     string    `json:"family_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	CreatedAt    time.Time `json:"created_at"`
}

type ChildProfile struct {
	ID               string    `json:"id"`
	FamilyID         string    `json:"family_id"`
	DisplayName      string    `json:"display_name"`
	EmergencyContact string    `json:"emergency_contact"`
	CreatedAt        time.Time `json:"created_at"`
}

type ScheduleRule struct {
	ID        string `json:"id"`
	ChildID   string `json:"child_id"`
	DayOfWeek int    `json:"day_of_week"` // 0 = Sunday .. 6 = Saturday
	StartTime string `json:"start_time"`  // "15:00"
	EndTime   string `json:"end_time"`    // "18:00"
	Timezone  string `json:"timezone"`    // "Local" or IANA name
	Enabled   bool   `json:"enabled"`
}

type Device struct {
	ID               string     `json:"id"`
	ChildID          string     `json:"child_id"`
	Platform         string     `json:"platform"` // WINDOWS, ANDROID
	DeviceName       string     `json:"device_name"`
	AuthTokenHash    string     `json:"-"`
	PairingCode      *string    `json:"pairing_code,omitempty"`
	PairingExpiresAt *time.Time `json:"pairing_expires_at,omitempty"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	Status           string     `json:"status"` // LOCKED, ACTIVE, TAMPERED, OFFLINE
	ValidUntil       *time.Time `json:"valid_until,omitempty"`
	RemainingSeconds int64      `json:"remaining_seconds,omitempty"`
	AccessReason     string     `json:"access_reason,omitempty"` // "GRANT" | "SCHEDULE" | ""
}

type ScreenTimeRequest struct {
	ID               string     `json:"id"`
	DeviceID         string     `json:"device_id"`
	ChildID          string     `json:"child_id"`
	ChildDisplayName string     `json:"child_display_name,omitempty"`
	DeviceName       string     `json:"device_name,omitempty"`
	Status           string     `json:"status"` // PENDING, APPROVED, DENIED, EXPIRED
	CreatedAt        time.Time  `json:"created_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
}

type TimeGrant struct {
	ID              string    `json:"id"`
	DeviceID        string    `json:"device_id"`
	GrantedByUserID *string   `json:"granted_by_user_id,omitempty"`
	RequestID       *string   `json:"request_id,omitempty"`
	ValidFrom       time.Time `json:"valid_from"`
	ValidUntil      time.Time `json:"valid_until"`
	CreatedAt       time.Time `json:"created_at"`
}

type PushSubscription struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Endpoint  string    `json:"endpoint"`
	P256dh    string    `json:"p256dh"`
	Auth      string    `json:"auth"`
	CreatedAt time.Time `json:"created_at"`
}

// AccessStatusResponse is returned to devices during the pre-popup check and polling
type AccessStatusResponse struct {
	Access            string     `json:"access"` // "ALLOWED" | "LOCKED"
	Reason            string     `json:"reason"` // "SCHEDULE" | "GRANT" | "NONE"
	ValidUntil        *time.Time `json:"valid_until,omitempty"`
	RemainingSeconds  int64      `json:"remaining_seconds"`
	NextScheduleStart *time.Time `json:"next_schedule_start,omitempty"`
	EmergencyContact  string     `json:"emergency_contact,omitempty"`
	ChildDisplayName  string     `json:"child_display_name,omitempty"`
}

// Request and Response payloads
type RegisterRequest struct {
	FamilyName  string `json:"family_name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string      `json:"token"`
	User  *FamilyUser `json:"user"`
}

type CreateChildRequest struct {
	DisplayName      string `json:"display_name"`
	EmergencyContact string `json:"emergency_contact"`
}

type GeneratePairingCodeResponse struct {
	PairingCode string    `json:"pairing_code"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type PairDeviceRequest struct {
	PairingCode string `json:"pairing_code"`
	Platform    string `json:"platform"` // "WINDOWS" | "ANDROID"
	DeviceName  string `json:"device_name"`
}

type PairDeviceResponse struct {
	DeviceID         string `json:"device_id"`
	AuthToken        string `json:"auth_token"`
	ChildID          string `json:"child_id"`
	ChildDisplayName string `json:"child_display_name"`
	EmergencyContact string `json:"emergency_contact"`
}

type ResolveScreenTimeRequest struct {
	Action       string     `json:"action"` // "GRANT" | "DENY"
	GrantMinutes int        `json:"grant_minutes,omitempty"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
}

type SetDeviceGrantRequest struct {
	Action       string     `json:"action"` // "SET" | "LOCK"
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	GrantMinutes int        `json:"grant_minutes,omitempty"`
}

type HeartbeatRequest struct {
	UptimeSeconds  int64  `json:"uptime_seconds"`
	ReportedStatus string `json:"reported_status"`
}
