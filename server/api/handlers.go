package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"screentime-server/auth"
	"screentime-server/models"
	"screentime-server/push"
	"screentime-server/realtime"
	"screentime-server/schedule"
)

type Server struct {
	db        *sql.DB
	hub       *realtime.Hub
	push      *push.PushManager
	router    *http.ServeMux
	staticDir string
}

func NewServer(db *sql.DB, staticDir string) *Server {
	s := &Server{
		db:        db,
		hub:       realtime.NewHub(),
		push:      push.NewPushManager(db),
		router:    http.NewServeMux(),
		staticDir: staticDir,
	}
	s.routes()
	s.startWatchdog()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Device-Token")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Auth
	s.router.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.router.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.router.HandleFunc("GET /api/auth/me", s.withUserAuth(s.handleMe))

	// Family Members
	s.router.HandleFunc("GET /api/family/members", s.withUserAuth(s.handleGetFamilyMembers))
	s.router.HandleFunc("POST /api/family/members", s.withUserAuth(s.handleAddFamilyMember))

	// Children
	s.router.HandleFunc("GET /api/children", s.withUserAuth(s.handleGetChildren))
	s.router.HandleFunc("POST /api/children", s.withUserAuth(s.handleCreateChild))
	s.router.HandleFunc("DELETE /api/children/{id}", s.withUserAuth(s.handleDeleteChild))

	// Schedules
	s.router.HandleFunc("GET /api/children/{id}/schedules", s.withUserAuth(s.handleGetChildSchedules))
	s.router.HandleFunc("POST /api/children/{id}/schedules", s.withUserAuth(s.handleSaveSchedule))
	s.router.HandleFunc("PUT /api/children/{id}/schedules", s.withUserAuth(s.handleSaveBatchSchedules))
	s.router.HandleFunc("DELETE /api/schedules/{id}", s.withUserAuth(s.handleDeleteSchedule))

	// Device Pairing
	s.router.HandleFunc("POST /api/children/{id}/pairing-code", s.withUserAuth(s.handleGeneratePairingCode))
	s.router.HandleFunc("POST /api/devices/pair", s.handlePairDevice)

	// Device Access Status (Pre-popup check & polling)
	s.router.HandleFunc("GET /api/devices/access-status", s.withDeviceAuth(s.handleGetAccessStatus))
	s.router.HandleFunc("POST /api/devices/heartbeat", s.withDeviceAuth(s.handleDeviceHeartbeat))
	s.router.HandleFunc("PUT /api/devices/{id}/grant", s.withUserAuth(s.handleSetDeviceGrant))

	// Requests
	s.router.HandleFunc("POST /api/requests", s.withDeviceAuth(s.handleCreateScreenTimeRequest))
	s.router.HandleFunc("GET /api/requests/pending", s.withUserAuth(s.handleGetPendingRequests))
	s.router.HandleFunc("POST /api/requests/{id}/resolve", s.handleResolveRequest)

	// Push & Realtime
	s.router.HandleFunc("GET /api/push/vapid-public-key", s.handleGetVapidPublicKey)
	s.router.HandleFunc("POST /api/push/subscribe", s.withUserAuth(s.handleSavePushSubscription))
	s.router.HandleFunc("/api/ws/admin", s.handleAdminWebSocket)
	s.router.HandleFunc("/api/ws/device", s.handleDeviceWebSocket)

	// Static SPA (Web Admin)
	s.router.HandleFunc("/", s.handleStaticSPA)
}

func (s *Server) handleStaticSPA(w http.ResponseWriter, r *http.Request) {
	if s.staticDir == "" {
		http.NotFound(w, r)
		return
	}

	// Clean path
	requestedPath := filepath.Clean(r.URL.Path)
	targetPath := filepath.Join(s.staticDir, requestedPath)

	info, err := os.Stat(targetPath)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, targetPath)
		return
	}

	// Check if file exists inside static directory directly (e.g. /favicon.ico)
	if strings.Contains(requestedPath, ".") && err == nil {
		http.ServeFile(w, r, targetPath)
		return
	}

	// Fallback to index.html for Angular SPA client-side routes (e.g. /login, /dashboard)
	indexPath := filepath.Join(s.staticDir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}

	http.NotFound(w, r)
}

// Background watchdog checking for missing device heartbeats
func (s *Server) startWatchdog() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			threshold := time.Now().Add(-3 * time.Minute)
			rows, err := s.db.Query(`
				SELECT d.id, d.device_name, cp.display_name, fu.family_id
				FROM devices d
				JOIN child_profiles cp ON d.child_id = cp.id
				JOIN families fu ON cp.family_id = fu.id
				WHERE d.status IN ('ACTIVE', 'LOCKED') AND d.last_seen_at < ?
			`, threshold)
			if err != nil {
				continue
			}

			for rows.Next() {
				var devID, devName, childName, familyID string
				if err := rows.Scan(&devID, &devName, &childName, &familyID); err == nil {
					// Mark as OFFLINE
					s.db.Exec("UPDATE devices SET status = 'OFFLINE' WHERE id = ?", devID)
					// Notify parents via Push
					s.push.SendToFamily(familyID, push.PushPayload{
						Title: "Device Offline Alert",
						Body:  fmt.Sprintf("%s's device (%s) has stopped checking in.", childName, devName),
						Tag:   "device-offline-" + devID,
					})
				}
			}
			rows.Close()
		}
	}()
}

// Context keys
type contextKey string

const (
	userClaimsKey   contextKey = "userClaims"
	deviceClaimsKey contextKey = "deviceClaims"
)

// Auth Middleware for Parents
func (s *Server) withUserAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := auth.ValidateUserToken(token)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userClaimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// Auth Middleware for Devices
func (s *Server) withDeviceAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			authHeader = r.Header.Get("X-Device-Token")
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := auth.ValidateDeviceToken(token)
		if err != nil {
			http.Error(w, `{"error":"invalid device token"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), deviceClaimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// Handler implementations

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.FamilyName == "" || req.DisplayName == "" {
		http.Error(w, `{"error":"all fields required"}`, http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error":"server error"}`, http.StatusInternalServerError)
		return
	}

	familyID := auth.GenerateUUID()
	userID := auth.GenerateUUID()

	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("INSERT INTO families (id, name) VALUES (?, ?)", familyID, req.FamilyName); err != nil {
		http.Error(w, `{"error":"failed to create family"}`, http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec(`
		INSERT INTO family_users (id, family_id, email, password_hash, display_name)
		VALUES (?, ?, ?, ?, ?)
	`, userID, familyID, strings.ToLower(req.Email), hash, req.DisplayName); err != nil {
		http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error":"commit failed"}`, http.StatusInternalServerError)
		return
	}

	token, _ := auth.GenerateUserToken(userID, familyID, req.Email, req.DisplayName)
	jsonResp(w, http.StatusOK, models.AuthResponse{
		Token: token,
		User: &models.FamilyUser{
			ID:          userID,
			FamilyID:    familyID,
			Email:       req.Email,
			DisplayName: req.DisplayName,
			CreatedAt:   time.Now(),
		},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	var user models.FamilyUser
	err := s.db.QueryRow(`
		SELECT id, family_id, email, password_hash, display_name, created_at
		FROM family_users
		WHERE email = ?
	`, strings.ToLower(req.Email)).Scan(&user.ID, &user.FamilyID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.CreatedAt)
	if err != nil || !auth.CheckPasswordHash(req.Password, user.PasswordHash) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	token, _ := auth.GenerateUserToken(user.ID, user.FamilyID, user.Email, user.DisplayName)
	jsonResp(w, http.StatusOK, models.AuthResponse{
		Token: token,
		User:  &user,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)
	var user models.FamilyUser
	var familyName string

	err := s.db.QueryRow(`
		SELECT fu.id, fu.family_id, fu.email, fu.display_name, fu.created_at, f.name
		FROM family_users fu
		JOIN families f ON fu.family_id = f.id
		WHERE fu.id = ?
	`, claims.UserID).Scan(&user.ID, &user.FamilyID, &user.Email, &user.DisplayName, &user.CreatedAt, &familyName)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"user":        user,
		"family_name": familyName,
	})
}

func (s *Server) handleGetFamilyMembers(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)
	rows, err := s.db.Query(`
		SELECT id, family_id, email, display_name, created_at
		FROM family_users
		WHERE family_id = ?
		ORDER BY created_at ASC
	`, claims.FamilyID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	members := make([]models.FamilyUser, 0)
	for rows.Next() {
		var u models.FamilyUser
		if err := rows.Scan(&u.ID, &u.FamilyID, &u.Email, &u.DisplayName, &u.CreatedAt); err == nil {
			members = append(members, u)
		}
	}
	jsonResp(w, http.StatusOK, members)
}

func (s *Server) handleAddFamilyMember(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)
	var req struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	hash, _ := auth.HashPassword(req.Password)
	newID := auth.GenerateUUID()
	_, err := s.db.Exec(`
		INSERT INTO family_users (id, family_id, email, password_hash, display_name)
		VALUES (?, ?, ?, ?, ?)
	`, newID, claims.FamilyID, strings.ToLower(req.Email), hash, req.DisplayName)
	if err != nil {
		http.Error(w, `{"error":"email already exists"}`, http.StatusConflict)
		return
	}

	jsonResp(w, http.StatusCreated, map[string]string{"id": newID})
}

func (s *Server) handleGetChildren(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)
	rows, err := s.db.Query(`
		SELECT id, family_id, display_name, emergency_contact, created_at
		FROM child_profiles
		WHERE family_id = ?
		ORDER BY created_at ASC
	`, claims.FamilyID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ChildWithDevices struct {
		models.ChildProfile
		Devices   []models.Device       `json:"devices"`
		Schedules []models.ScheduleRule `json:"schedules"`
	}

	now := time.Now()
	children := make([]ChildWithDevices, 0)
	for rows.Next() {
		var c models.ChildProfile
		if err := rows.Scan(&c.ID, &c.FamilyID, &c.DisplayName, &c.EmergencyContact, &c.CreatedAt); err == nil {
			devRows, _ := s.db.Query(`
				SELECT id, child_id, platform, device_name, pairing_code, last_seen_at, status
				FROM devices WHERE child_id = ? AND status != 'PENDING'
			`, c.ID)
			devices := make([]models.Device, 0)
			if devRows != nil {
				for devRows.Next() {
					var d models.Device
					devRows.Scan(&d.ID, &d.ChildID, &d.Platform, &d.DeviceName, &d.PairingCode, &d.LastSeenAt, &d.Status)
					access, reason, validUntil, remaining := s.evaluateDeviceAccess(d.ID, c.ID, now)
					if access == "ALLOWED" {
						d.ValidUntil = validUntil
						d.RemainingSeconds = remaining
						d.AccessReason = reason
						if d.Status == "LOCKED" {
							d.Status = "ACTIVE"
						}
					} else if d.Status == "ACTIVE" {
						d.Status = "LOCKED"
					}
					devices = append(devices, d)
				}
				devRows.Close()
			}

			schedRows, _ := s.db.Query(`
				SELECT id, child_id, day_of_week, start_time, end_time, timezone, enabled
				FROM schedule_rules WHERE child_id = ?
				ORDER BY day_of_week, start_time
			`, c.ID)
			schedules := make([]models.ScheduleRule, 0)
			if schedRows != nil {
				for schedRows.Next() {
					var sc models.ScheduleRule
					schedRows.Scan(&sc.ID, &sc.ChildID, &sc.DayOfWeek, &sc.StartTime, &sc.EndTime, &sc.Timezone, &sc.Enabled)
					schedules = append(schedules, sc)
				}
				schedRows.Close()
			}

			children = append(children, ChildWithDevices{
				ChildProfile: c,
				Devices:      devices,
				Schedules:    schedules,
			})
		}
	}
	jsonResp(w, http.StatusOK, children)
}

func (s *Server) handleCreateChild(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)
	var req models.CreateChildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DisplayName == "" {
		http.Error(w, `{"error":"display_name is required"}`, http.StatusBadRequest)
		return
	}

	childID := auth.GenerateUUID()
	_, err := s.db.Exec(`
		INSERT INTO child_profiles (id, family_id, display_name, emergency_contact)
		VALUES (?, ?, ?, ?)
	`, childID, claims.FamilyID, req.DisplayName, req.EmergencyContact)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	jsonResp(w, http.StatusCreated, map[string]string{"id": childID})
}

func (s *Server) handleDeleteChild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)

	_, err := s.db.Exec("DELETE FROM child_profiles WHERE id = ? AND family_id = ?", id, claims.FamilyID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleGetChildSchedules(w http.ResponseWriter, r *http.Request) {
	childID := r.PathValue("id")
	rows, err := s.db.Query(`
		SELECT id, child_id, day_of_week, start_time, end_time, timezone, enabled
		FROM schedule_rules
		WHERE child_id = ?
		ORDER BY day_of_week, start_time
	`, childID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := make([]models.ScheduleRule, 0)
	for rows.Next() {
		var r models.ScheduleRule
		if err := rows.Scan(&r.ID, &r.ChildID, &r.DayOfWeek, &r.StartTime, &r.EndTime, &r.Timezone, &r.Enabled); err == nil {
			list = append(list, r)
		}
	}
	jsonResp(w, http.StatusOK, list)
}

func (s *Server) handleSaveBatchSchedules(w http.ResponseWriter, r *http.Request) {
	childID := r.PathValue("id")
	var payload struct {
		Rules []models.ScheduleRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, `{"error":"db error: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Clear existing rules for this child to avoid duplicates
	if _, err := tx.Exec("DELETE FROM schedule_rules WHERE child_id = ?", childID); err != nil {
		http.Error(w, `{"error":"failed to clear old rules: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	for _, rule := range payload.Rules {
		ruleID := rule.ID
		if ruleID == "" {
			ruleID = auth.GenerateUUID()
		}
		tz := rule.Timezone
		if tz == "" {
			tz = "Local"
		}
		st := rule.StartTime
		if st == "" {
			st = "15:00"
		}
		et := rule.EndTime
		if et == "" {
			et = "18:00"
		}

		_, err := tx.Exec(`
			INSERT INTO schedule_rules (id, child_id, day_of_week, start_time, end_time, timezone, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, ruleID, childID, rule.DayOfWeek, st, et, tz, rule.Enabled)
		if err != nil {
			http.Error(w, `{"error":"failed to insert rule: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error":"commit failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Notify all connected devices of this child so they can re-evaluate schedule immediately!
	devRows, _ := s.db.Query("SELECT id FROM devices WHERE child_id = ?", childID)
	if devRows != nil {
		for devRows.Next() {
			var devID string
			if devRows.Scan(&devID) == nil {
				s.hub.SendToDevice(devID, map[string]interface{}{
					"type": "SCHEDULE_UPDATED",
				})
			}
		}
		devRows.Close()
	}

	jsonResp(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleSaveSchedule(w http.ResponseWriter, r *http.Request) {
	childID := r.PathValue("id")
	var req models.ScheduleRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	ruleID := req.ID
	if ruleID == "" {
		ruleID = auth.GenerateUUID()
	}

	if req.Timezone == "" {
		req.Timezone = "Local"
	}
	if req.StartTime == "" {
		req.StartTime = "15:00"
	}
	if req.EndTime == "" {
		req.EndTime = "18:00"
	}

	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Delete any existing rule for this day_of_week for this child to avoid duplicates
	tx.Exec("DELETE FROM schedule_rules WHERE child_id = ? AND day_of_week = ?", childID, req.DayOfWeek)

	_, err = tx.Exec(`
		INSERT INTO schedule_rules (id, child_id, day_of_week, start_time, end_time, timezone, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, ruleID, childID, req.DayOfWeek, req.StartTime, req.EndTime, req.Timezone, req.Enabled)
	if err != nil {
		http.Error(w, `{"error":"failed to save schedule: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error":"commit error: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Notify all connected devices of this child so they can re-evaluate schedule immediately!
	devRows, _ := s.db.Query("SELECT id FROM devices WHERE child_id = ?", childID)
	if devRows != nil {
		for devRows.Next() {
			var devID string
			if devRows.Scan(&devID) == nil {
				s.hub.SendToDevice(devID, map[string]interface{}{
					"type": "SCHEDULE_UPDATED",
				})
			}
		}
		devRows.Close()
	}

	jsonResp(w, http.StatusOK, map[string]string{"id": ruleID})
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, err := s.db.Exec("DELETE FROM schedule_rules WHERE id = ?", id)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleGeneratePairingCode(w http.ResponseWriter, r *http.Request) {
	childID := r.PathValue("id")
	code, err := auth.GenerateSecureCode(6)
	if err != nil {
		http.Error(w, `{"error":"code generator failed"}`, http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	// Upsert a pending device placeholder with this pairing code
	deviceID := auth.GenerateUUID()
	_, err = s.db.Exec(`
		INSERT INTO devices (id, child_id, platform, device_name, auth_token_hash, pairing_code, pairing_expires_at, status)
		VALUES (?, ?, 'PENDING', 'New Device', '', ?, ?, 'PENDING')
	`, deviceID, childID, code, expiresAt)
	if err != nil {
		http.Error(w, `{"error":"failed to generate pairing record"}`, http.StatusInternalServerError)
		return
	}

	jsonResp(w, http.StatusOK, models.GeneratePairingCodeResponse{
		PairingCode: code,
		ExpiresAt:   expiresAt,
	})
}

func (s *Server) handlePairDevice(w http.ResponseWriter, r *http.Request) {
	var req models.PairDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PairingCode == "" {
		http.Error(w, `{"error":"pairing_code is required"}`, http.StatusBadRequest)
		return
	}

	var devID, childID, childName, emergencyContact string
	var expiresAt time.Time

	err := s.db.QueryRow(`
		SELECT d.id, d.child_id, d.pairing_expires_at, cp.display_name, cp.emergency_contact
		FROM devices d
		JOIN child_profiles cp ON d.child_id = cp.id
		WHERE d.pairing_code = ?
	`, strings.ToUpper(strings.TrimSpace(req.PairingCode))).Scan(&devID, &childID, &expiresAt, &childName, &emergencyContact)

	if err != nil || time.Now().After(expiresAt) {
		http.Error(w, `{"error":"invalid or expired pairing code"}`, http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateDeviceToken(devID, childID)
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	tokenHash, _ := auth.HashPassword(token)
	now := time.Now()

	s.db.Exec(`
		UPDATE devices
		SET platform = ?, device_name = ?, auth_token_hash = ?, pairing_code = NULL, pairing_expires_at = NULL,
		    status = 'LOCKED', last_seen_at = ?
		WHERE id = ?
	`, req.Platform, req.DeviceName, tokenHash, now, devID)

	jsonResp(w, http.StatusOK, models.PairDeviceResponse{
		DeviceID:         devID,
		AuthToken:        token,
		ChildID:          childID,
		ChildDisplayName: childName,
		EmergencyContact: emergencyContact,
	})
}

// evaluateDeviceAccess computes the authoritative access state, reason, and expiry for a device.
func (s *Server) evaluateDeviceAccess(deviceID, childID string, now time.Time) (string, string, *time.Time, int64) {
	nowUTC := now.UTC()
	var grantUntil time.Time
	var isOverride bool
	hasActiveGrant := false

	err := s.db.QueryRow(`
		SELECT valid_until, is_override
		FROM time_grants
		WHERE device_id = ? AND valid_until > ?
		ORDER BY valid_until DESC
		LIMIT 1
	`, deviceID, nowUTC).Scan(&grantUntil, &isOverride)
	if err == nil {
		hasActiveGrant = true
		if isOverride {
			untilUTC := grantUntil.UTC()
			return "ALLOWED", "GRANT", &untilUTC, int64(grantUntil.Sub(nowUTC).Seconds())
		}
	}

	evalResult, schedErr := schedule.EvaluateChildSchedule(s.db, childID, now)
	hasActiveSchedule := schedErr == nil && evalResult != nil && evalResult.IsUnrestricted && evalResult.ValidUntil != nil

	if hasActiveSchedule && evalResult.RuleStart != nil {
		// Check whether a parent explicitly overrode/locked the device during this schedule window
		var overrideCount int
		_ = s.db.QueryRow(`
			SELECT COUNT(1)
			FROM time_grants
			WHERE device_id = ? AND is_override = 1 AND valid_from >= ? AND valid_until <= ?
		`, deviceID, evalResult.RuleStart.UTC(), nowUTC).Scan(&overrideCount)
		if overrideCount > 0 {
			hasActiveSchedule = false
		}
	}

	if hasActiveGrant && hasActiveSchedule {
		if grantUntil.After(*evalResult.ValidUntil) {
			untilUTC := grantUntil.UTC()
			return "ALLOWED", "GRANT", &untilUTC, int64(grantUntil.Sub(nowUTC).Seconds())
		}
		return "ALLOWED", "SCHEDULE", evalResult.ValidUntil, evalResult.RemainingSeconds
	}

	if hasActiveGrant {
		untilUTC := grantUntil.UTC()
		return "ALLOWED", "GRANT", &untilUTC, int64(grantUntil.Sub(nowUTC).Seconds())
	}

	if hasActiveSchedule {
		return "ALLOWED", "SCHEDULE", evalResult.ValidUntil, evalResult.RemainingSeconds
	}

	return "LOCKED", "NONE", nil, 0
}

// Pre-popup check & live polling
func (s *Server) handleGetAccessStatus(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(deviceClaimsKey).(*auth.DeviceClaims)
	now := time.Now()

	// Update device last_seen_at
	s.db.Exec("UPDATE devices SET last_seen_at = ? WHERE id = ?", now, claims.DeviceID)

	var childName, emergencyContact string
	s.db.QueryRow(`
		SELECT display_name, emergency_contact FROM child_profiles WHERE id = ?
	`, claims.ChildID).Scan(&childName, &emergencyContact)

	access, reason, validUntil, remaining := s.evaluateDeviceAccess(claims.DeviceID, claims.ChildID, now)
	resp := models.AccessStatusResponse{
		Access:           access,
		Reason:           reason,
		ValidUntil:       validUntil,
		RemainingSeconds: remaining,
		EmergencyContact: emergencyContact,
		ChildDisplayName: childName,
	}

	if access == "ALLOWED" {
		s.db.Exec("UPDATE devices SET status = 'ACTIVE' WHERE id = ?", claims.DeviceID)
	} else {
		s.db.Exec("UPDATE devices SET status = 'LOCKED' WHERE id = ?", claims.DeviceID)
	}
	jsonResp(w, http.StatusOK, resp)
}

func (s *Server) handleCreateScreenTimeRequest(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(deviceClaimsKey).(*auth.DeviceClaims)

	// Check if already pending
	var existingID string
	err := s.db.QueryRow(`
		SELECT id FROM screen_time_requests
		WHERE device_id = ? AND status = 'PENDING'
	`, claims.DeviceID).Scan(&existingID)

	if err == nil {
		// Existing pending request already in flight
		jsonResp(w, http.StatusOK, map[string]string{
			"status":     "PENDING",
			"request_id": existingID,
		})
		return
	}

	reqID := auth.GenerateUUID()
	now := time.Now()

	_, err = s.db.Exec(`
		INSERT INTO screen_time_requests (id, device_id, child_id, status, created_at)
		VALUES (?, ?, ?, 'PENDING', ?)
	`, reqID, claims.DeviceID, claims.ChildID, now)
	if err != nil {
		http.Error(w, `{"error":"failed to create request"}`, http.StatusInternalServerError)
		return
	}

	// Fetch details for notification
	var childName, devName, familyID string
	s.db.QueryRow(`
		SELECT cp.display_name, d.device_name, cp.family_id
		FROM devices d
		JOIN child_profiles cp ON d.child_id = cp.id
		WHERE d.id = ?
	`, claims.DeviceID).Scan(&childName, &devName, &familyID)

	// 1. WebSocket broadcast to connected parents
	s.hub.BroadcastToFamily(familyID, map[string]interface{}{
		"type":               "SCREEN_TIME_REQUESTED",
		"request_id":         reqID,
		"device_id":          claims.DeviceID,
		"child_id":           claims.ChildID,
		"child_display_name": childName,
		"device_name":        devName,
		"created_at":         now,
	})

	// 2. Web Push notification to parents' phones
	actionToken, _ := auth.GeneratePushActionToken(familyID, reqID)
	s.push.SendToFamily(familyID, push.PushPayload{
		Title: "Screen Time Request",
		Body:  fmt.Sprintf("%s is requesting screen time on %s", childName, devName),
		Tag:   "screen-time-request-" + reqID,
		Data: map[string]interface{}{
			"type":         "SCREEN_TIME_REQUEST",
			"request_id":   reqID,
			"device_id":    claims.DeviceID,
			"child_name":   childName,
			"device_name":  devName,
			"action_token": actionToken,
		},
	})

	jsonResp(w, http.StatusCreated, map[string]string{
		"status":     "PENDING",
		"request_id": reqID,
	})
}

func (s *Server) handleGetPendingRequests(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)
	rows, err := s.db.Query(`
		SELECT r.id, r.device_id, r.child_id, cp.display_name, d.device_name, r.status, r.created_at
		FROM screen_time_requests r
		JOIN child_profiles cp ON r.child_id = cp.id
		JOIN devices d ON r.device_id = d.id
		WHERE cp.family_id = ? AND r.status = 'PENDING'
		ORDER BY r.created_at DESC
	`, claims.FamilyID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := make([]models.ScreenTimeRequest, 0)
	for rows.Next() {
		var req models.ScreenTimeRequest
		if err := rows.Scan(&req.ID, &req.DeviceID, &req.ChildID, &req.ChildDisplayName, &req.DeviceName, &req.Status, &req.CreatedAt); err == nil {
			list = append(list, req)
		}
	}
	jsonResp(w, http.StatusOK, list)
}

func (s *Server) handleResolveRequest(w http.ResponseWriter, r *http.Request) {
	reqID := r.PathValue("id")

	// Authenticate via either standard parent Bearer token OR signed push notification action token
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	rawToken := strings.TrimPrefix(authHeader, "Bearer ")

	var familyID, actorName string
	var userIDPtr *string

	if userClaims, err := auth.ValidateUserToken(rawToken); err == nil {
		familyID = userClaims.FamilyID
		actorName = userClaims.DisplayName
		uid := userClaims.UserID
		userIDPtr = &uid
	} else if pushClaims, err := auth.ValidatePushActionToken(rawToken); err == nil && pushClaims.RequestID == reqID {
		familyID = pushClaims.FamilyID
		actorName = "Parent (Notification)"
	} else {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
		return
	}

	var payload models.ResolveScreenTimeRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	var devID, childID string
	err := s.db.QueryRow(`
		SELECT r.device_id, r.child_id
		FROM screen_time_requests r
		JOIN child_profiles cp ON r.child_id = cp.id
		WHERE r.id = ? AND cp.family_id = ? AND r.status = 'PENDING'
	`, reqID, familyID).Scan(&devID, &childID)

	if err != nil {
		http.Error(w, `{"error":"request not found or already resolved"}`, http.StatusNotFound)
		return
	}

	now := time.Now().UTC()
	if payload.Action == "GRANT" {
		var validUntil time.Time
		if payload.ValidUntil != nil && payload.ValidUntil.After(now) {
			validUntil = payload.ValidUntil.UTC()
		} else {
			if payload.GrantMinutes <= 0 {
				payload.GrantMinutes = 30
			}
			validUntil = now.Add(time.Duration(payload.GrantMinutes) * time.Minute).UTC()
		}
		grantID := auth.GenerateUUID()

		// Expire any prior active grants for this device so the new end time is authoritative
		s.db.Exec("UPDATE time_grants SET valid_until = ? WHERE device_id = ? AND valid_until > ?", now, devID, now)

		s.db.Exec(`
			INSERT INTO time_grants (id, device_id, granted_by_user_id, request_id, valid_from, valid_until, is_override)
			VALUES (?, ?, ?, ?, ?, ?, 1)
		`, grantID, devID, userIDPtr, reqID, now, validUntil)

		s.db.Exec("UPDATE screen_time_requests SET status = 'APPROVED', resolved_at = ? WHERE id = ?", now, reqID)
		s.db.Exec("UPDATE devices SET status = 'ACTIVE' WHERE id = ?", devID)

		remainingSecs := int64(validUntil.Sub(now).Seconds())

		// Realtime signal to the managed device to immediately unlock!
		s.hub.SendToDevice(devID, map[string]interface{}{
			"type":              "ACCESS_UPDATE",
			"access":            "ALLOWED",
			"reason":            "GRANT",
			"valid_until":       validUntil,
			"remaining_seconds": remainingSecs,
		})

		// Notify other parents
		s.hub.BroadcastToFamily(familyID, map[string]interface{}{
			"type":        "REQUEST_RESOLVED",
			"request_id":  reqID,
			"status":      "APPROVED",
			"granted_by":  actorName,
			"valid_until": validUntil,
		})
	} else {
		s.db.Exec("UPDATE screen_time_requests SET status = 'DENIED', resolved_at = ? WHERE id = ?", now, reqID)

		// Signal device that request was denied
		s.hub.SendToDevice(devID, map[string]interface{}{
			"type":   "REQUEST_DENIED",
			"access": "LOCKED",
		})

		s.hub.BroadcastToFamily(familyID, map[string]interface{}{
			"type":       "REQUEST_RESOLVED",
			"request_id": reqID,
			"status":     "DENIED",
			"denied_by":  actorName,
		})
	}

	jsonResp(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func (s *Server) handleSetDeviceGrant(w http.ResponseWriter, r *http.Request) {
	devID := r.PathValue("id")
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)

	var payload models.SetDeviceGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// Verify device belongs to this parent's family
	var childID string
	err := s.db.QueryRow(`
		SELECT d.child_id
		FROM devices d
		JOIN child_profiles cp ON d.child_id = cp.id
		WHERE d.id = ? AND cp.family_id = ?
	`, devID, claims.FamilyID).Scan(&childID)
	if err != nil {
		http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
		return
	}

	now := time.Now().UTC()

	// Expire any existing active grants for this device
	s.db.Exec("UPDATE time_grants SET valid_until = ? WHERE device_id = ? AND valid_until > ?", now, devID, now)

	if payload.Action == "LOCK" {
		// Insert an immediate override record so even an active recurring schedule window is locked
		overrideID := auth.GenerateUUID()
		s.db.Exec(`
			INSERT INTO time_grants (id, device_id, granted_by_user_id, valid_from, valid_until, is_override)
			VALUES (?, ?, ?, ?, ?, 1)
		`, overrideID, devID, claims.UserID, now, now)

		s.db.Exec("UPDATE screen_time_requests SET status = 'DENIED', resolved_at = ? WHERE device_id = ? AND status = 'PENDING'", now, devID)
		s.db.Exec("UPDATE devices SET status = 'LOCKED' WHERE id = ?", devID)

		// Signal device to lock immediately (compatible with both existing and updated clients)
		s.hub.SendToDevice(devID, map[string]interface{}{
			"type":   "REQUEST_DENIED",
			"access": "LOCKED",
		})

		s.hub.BroadcastToFamily(claims.FamilyID, map[string]interface{}{
			"type":      "DEVICE_GRANT_UPDATED",
			"device_id": devID,
			"access":    "LOCKED",
		})

		jsonResp(w, http.StatusOK, map[string]interface{}{
			"status": "locked",
		})
		return
	}

	var validUntil time.Time
	if payload.ValidUntil != nil {
		validUntil = payload.ValidUntil.UTC()
	} else if payload.GrantMinutes > 0 {
		validUntil = now.Add(time.Duration(payload.GrantMinutes) * time.Minute).UTC()
	} else {
		http.Error(w, `{"error":"valid_until or grant_minutes is required"}`, http.StatusBadRequest)
		return
	}

	if !validUntil.After(now) {
		http.Error(w, `{"error":"end time must be in the future"}`, http.StatusBadRequest)
		return
	}

	grantID := auth.GenerateUUID()
	_, err = s.db.Exec(`
		INSERT INTO time_grants (id, device_id, granted_by_user_id, valid_from, valid_until, is_override)
		VALUES (?, ?, ?, ?, ?, 1)
	`, grantID, devID, claims.UserID, now, validUntil)
	if err != nil {
		http.Error(w, `{"error":"failed to save end time"}`, http.StatusInternalServerError)
		return
	}

	// Also mark any pending screen time request for this device as approved
	s.db.Exec("UPDATE screen_time_requests SET status = 'APPROVED', resolved_at = ? WHERE device_id = ? AND status = 'PENDING'", now, devID)
	s.db.Exec("UPDATE devices SET status = 'ACTIVE' WHERE id = ?", devID)

	remainingSecs := int64(validUntil.Sub(now).Seconds())

	// Signal connected device to unlock or update its remaining countdown immediately
	s.hub.SendToDevice(devID, map[string]interface{}{
		"type":              "ACCESS_UPDATE",
		"access":            "ALLOWED",
		"reason":            "GRANT",
		"valid_until":       validUntil,
		"remaining_seconds": remainingSecs,
	})

	s.hub.BroadcastToFamily(claims.FamilyID, map[string]interface{}{
		"type":              "DEVICE_GRANT_UPDATED",
		"device_id":         devID,
		"access":            "ALLOWED",
		"valid_until":       validUntil,
		"remaining_seconds": remainingSecs,
	})

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"status":            "updated",
		"valid_until":       validUntil,
		"remaining_seconds": remainingSecs,
	})
}

func (s *Server) handleDeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(deviceClaimsKey).(*auth.DeviceClaims)
	var req models.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	now := time.Now()
	s.db.Exec(`
		INSERT INTO heartbeats (device_id, last_uptime_seconds, last_ping_at, reported_status)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			last_uptime_seconds = excluded.last_uptime_seconds,
			last_ping_at = excluded.last_ping_at,
			reported_status = excluded.reported_status
	`, claims.DeviceID, req.UptimeSeconds, now, req.ReportedStatus)

	s.db.Exec("UPDATE devices SET last_seen_at = ? WHERE id = ?", now, claims.DeviceID)
	jsonResp(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetVapidPublicKey(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, http.StatusOK, map[string]string{
		"public_key": s.push.VapidPublicKey,
	})
}

func (s *Server) handleSavePushSubscription(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(userClaimsKey).(*auth.UserClaims)
	var req struct {
		Endpoint string `json:"endpoint"`
		P256dh   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Endpoint == "" {
		http.Error(w, `{"error":"invalid subscription"}`, http.StatusBadRequest)
		return
	}

	subID := auth.GenerateUUID()
	s.db.Exec(`
		INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			user_id = excluded.user_id,
			p256dh = excluded.p256dh,
			auth = excluded.auth
	`, subID, claims.UserID, req.Endpoint, req.P256dh, req.Auth)

	jsonResp(w, http.StatusOK, map[string]string{"status": "subscribed"})
}

func (s *Server) handleAdminWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	claims, err := auth.ValidateUserToken(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.hub.Upgrade(w, r)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}

	s.hub.RegisterParent(claims.FamilyID, conn)
	defer s.hub.UnregisterParent(claims.FamilyID, conn)

	// Keep alive / read loop
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (s *Server) handleDeviceWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	claims, err := auth.ValidateDeviceToken(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.hub.Upgrade(w, r)
	if err != nil {
		log.Printf("WS device upgrade error: %v", err)
		return
	}

	s.hub.RegisterDevice(claims.DeviceID, conn)
	defer s.hub.UnregisterDevice(claims.DeviceID, conn)

	// Keep alive / read loop
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func jsonResp(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

