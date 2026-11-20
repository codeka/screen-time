package api_test

import (
	"bytes"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"screentime-server/api"
	"screentime-server/auth"
	"screentime-server/db"
	"screentime-server/models"
)

func setupTestServer(t *testing.T) (*api.Server, func()) {
	tmpFile := "test_screentime_" + time.Now().Format("150405.000") + ".db"
	database, err := db.InitDB(tmpFile)
	if err != nil {
		t.Fatalf("Failed to init test db: %v", err)
	}

	server := api.NewServer(database, "")

	cleanup := func() {
		database.Close()
		os.Remove(tmpFile)
		os.Remove(tmpFile + "-wal")
		os.Remove(tmpFile + "-shm")
	}

	return server, cleanup
}

func setupTestServerWithDB(t *testing.T) (*api.Server, *sql.DB, func()) {
	tmpFile := "test_screentime_" + time.Now().Format("150405.000") + ".db"
	database, err := db.InitDB(tmpFile)
	if err != nil {
		t.Fatalf("Failed to init test db: %v", err)
	}

	server := api.NewServer(database, "")

	cleanup := func() {
		database.Close()
		os.Remove(tmpFile)
		os.Remove(tmpFile + "-wal")
		os.Remove(tmpFile + "-shm")
	}

	return server, database, cleanup
}

func TestCompleteWorkflow(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Register Family & Parent
	regPayload := `{"family_name":"The Millers","display_name":"Dad","email":"dad@example.com","password":"secretpassword123"}`
	req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewBufferString(regPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Register failed: code %d, body: %s", w.Code, w.Body.String())
	}

	var authResp models.AuthResponse
	json.NewDecoder(w.Body).Decode(&authResp)
	parentToken := authResp.Token
	if parentToken == "" {
		t.Fatal("Expected parent token")
	}

	// 2. Create Child Profile
	childPayload := `{"display_name":"Lucas","emergency_contact":"555-0199"}`
	req = httptest.NewRequest("POST", "/api/children", bytes.NewBufferString(childPayload))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Create child failed: code %d, body: %s", w.Code, w.Body.String())
	}
	var childResp map[string]string
	json.NewDecoder(w.Body).Decode(&childResp)
	childID := childResp["id"]

	// 3. Generate Pairing Code
	req = httptest.NewRequest("POST", "/api/children/"+childID+"/pairing-code", nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Generate pairing code failed: code %d, body: %s", w.Code, w.Body.String())
	}
	var pairCodeResp models.GeneratePairingCodeResponse
	json.NewDecoder(w.Body).Decode(&pairCodeResp)
	if pairCodeResp.PairingCode == "" {
		t.Fatal("Pairing code empty")
	}

	// 4. Pair Windows Device
	pairDevPayload, _ := json.Marshal(models.PairDeviceRequest{
		PairingCode: pairCodeResp.PairingCode,
		Platform:    "WINDOWS",
		DeviceName:  "Lucas PC",
	})
	req = httptest.NewRequest("POST", "/api/devices/pair", bytes.NewReader(pairDevPayload))
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Pair device failed: code %d, body: %s", w.Code, w.Body.String())
	}
	var devResp models.PairDeviceResponse
	json.NewDecoder(w.Body).Decode(&devResp)
	deviceToken := devResp.AuthToken
	if deviceToken == "" {
		t.Fatal("Expected device auth token")
	}

	// 5. Pre-Popup Check: Device should initially be LOCKED
	req = httptest.NewRequest("GET", "/api/devices/access-status", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	var accessResp models.AccessStatusResponse
	json.NewDecoder(w.Body).Decode(&accessResp)
	if accessResp.Access != "LOCKED" {
		t.Fatalf("Expected access to be LOCKED, got: %s", accessResp.Access)
	}

	// 6. Child clicks "Request Screen Time"
	req = httptest.NewRequest("POST", "/api/requests", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Screen time request failed: code %d, body: %s", w.Code, w.Body.String())
	}
	var reqResp map[string]string
	json.NewDecoder(w.Body).Decode(&reqResp)
	requestID := reqResp["request_id"]

	// 7. Parent views pending requests
	req = httptest.NewRequest("GET", "/api/requests/pending", nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	var pendingList []models.ScreenTimeRequest
	json.NewDecoder(w.Body).Decode(&pendingList)
	if len(pendingList) != 1 || pendingList[0].ID != requestID {
		t.Fatalf("Expected 1 pending request with id %s, got %v", requestID, pendingList)
	}

	// 8. Parent approves with 45 minutes
	resolvePayload := `{"action":"GRANT","grant_minutes":45}`
	req = httptest.NewRequest("POST", "/api/requests/"+requestID+"/resolve", bytes.NewBufferString(resolvePayload))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Resolve request failed: code %d, body: %s", w.Code, w.Body.String())
	}

	// 9. Device polls access status again: Should now be ALLOWED (Reason: GRANT)
	req = httptest.NewRequest("GET", "/api/devices/access-status", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&accessResp)
	if accessResp.Access != "ALLOWED" || accessResp.Reason != "GRANT" {
		t.Fatalf("Expected ALLOWED via GRANT, got: %s / %s", accessResp.Access, accessResp.Reason)
	}
	if accessResp.RemainingSeconds <= 0 {
		t.Fatalf("Expected positive remaining seconds, got: %d", accessResp.RemainingSeconds)
	}

	// 10. Device sends heartbeat
	heartbeatPayload := `{"uptime_seconds":3600,"reported_status":"ACTIVE"}`
	req = httptest.NewRequest("POST", "/api/devices/heartbeat", bytes.NewBufferString(heartbeatPayload))
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Heartbeat failed: %d, %s", w.Code, w.Body.String())
	}

	// 11. Admin checks GET /api/children and sees active device expiry time
	req = httptest.NewRequest("GET", "/api/children", nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Get children failed: %d, %s", w.Code, w.Body.String())
	}
	var childrenResp []struct {
		models.ChildProfile
		Devices []models.Device `json:"devices"`
	}
	json.NewDecoder(w.Body).Decode(&childrenResp)
	if len(childrenResp) != 1 || len(childrenResp[0].Devices) != 1 {
		t.Fatalf("Expected 1 child with 1 paired device, got %+v", childrenResp)
	}
	if childrenResp[0].Devices[0].ValidUntil == nil || childrenResp[0].Devices[0].RemainingSeconds <= 0 {
		t.Fatalf("Expected device ValidUntil and positive RemainingSeconds, got %+v", childrenResp[0].Devices[0])
	}

	// 12. Parent edits end time on the device (shortens from 45m to 15m from now)
	newEnd := time.Now().Add(15 * time.Minute).UTC().Truncate(time.Second)
	editBytes, _ := json.Marshal(models.SetDeviceGrantRequest{
		Action:     "SET",
		ValidUntil: &newEnd,
	})
	req = httptest.NewRequest("PUT", "/api/devices/"+devResp.DeviceID+"/grant", bytes.NewReader(editBytes))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Edit device grant failed: %d, %s", w.Code, w.Body.String())
	}

	// Verify device access-status reflects shortened 15m expiry (<= 905s) instead of old 45m expiry
	req = httptest.NewRequest("GET", "/api/devices/access-status", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var shortenedResp models.AccessStatusResponse
	json.NewDecoder(w.Body).Decode(&shortenedResp)
	if shortenedResp.Access != "ALLOWED" || shortenedResp.RemainingSeconds > 905 || shortenedResp.RemainingSeconds < 850 {
		t.Fatalf("Expected shortened remaining seconds ~900, got %+v", shortenedResp)
	}

	// 13. Parent locks device immediately via PUT /api/devices/{id}/grant
	req = httptest.NewRequest("PUT", "/api/devices/"+devResp.DeviceID+"/grant", bytes.NewBufferString(`{"action":"LOCK"}`))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Lock device failed: %d, %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/devices/access-status", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var lockedResp models.AccessStatusResponse
	json.NewDecoder(w.Body).Decode(&lockedResp)
	if lockedResp.Access != "LOCKED" {
		t.Fatalf("Expected LOCKED after manual lock, got %+v", lockedResp)
	}

	// 14. Child requests again, resolved via Push Action Token with custom ValidUntil
	req = httptest.NewRequest("POST", "/api/requests", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var reqResp2 map[string]string
	json.NewDecoder(w.Body).Decode(&reqResp2)
	reqID2 := reqResp2["request_id"]

	pushToken, err := auth.GeneratePushActionToken(authResp.User.FamilyID, reqID2)
	if err != nil {
		t.Fatalf("Failed to generate push action token: %v", err)
	}
	customUntil := time.Now().Add(90 * time.Minute).UTC()
	customPayload, _ := json.Marshal(models.ResolveScreenTimeRequest{
		Action:     "GRANT",
		ValidUntil: &customUntil,
	})
	req = httptest.NewRequest("POST", "/api/requests/"+reqID2+"/resolve", bytes.NewReader(customPayload))
	req.Header.Set("Authorization", "Bearer "+pushToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Resolve with push token + custom ValidUntil failed: %d, %s", w.Code, w.Body.String())
	}

	// 15. Child clicks "Request More Time" on warning notification while 90m is still remaining;
	// parent grants +15m -> time stacks onto existing 90m (~105m = ~6300s)
	req = httptest.NewRequest("POST", "/api/requests", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Extension request failed: %d, %s", w.Code, w.Body.String())
	}
	var reqResp3 map[string]string
	json.NewDecoder(w.Body).Decode(&reqResp3)
	reqID3 := reqResp3["request_id"]

	req = httptest.NewRequest("POST", "/api/requests/"+reqID3+"/resolve", bytes.NewBufferString(`{"action":"GRANT","grant_minutes":15}`))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Grant extension failed: %d, %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/devices/access-status", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var stackedResp models.AccessStatusResponse
	json.NewDecoder(w.Body).Decode(&stackedResp)
	if stackedResp.Access != "ALLOWED" || stackedResp.RemainingSeconds < 6200 {
		t.Fatalf("Expected stacked remaining seconds ~6300s (90m + 15m), got %+v", stackedResp)
	}

	// 16. Child requests more time again while still active; parent DENIES -> remaining time is preserved
	req = httptest.NewRequest("POST", "/api/requests", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var reqResp4 map[string]string
	json.NewDecoder(w.Body).Decode(&reqResp4)
	reqID4 := reqResp4["request_id"]

	req = httptest.NewRequest("POST", "/api/requests/"+reqID4+"/resolve", bytes.NewBufferString(`{"action":"DENY"}`))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Deny extension failed: %d, %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/devices/access-status", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var afterDenyResp models.AccessStatusResponse
	json.NewDecoder(w.Body).Decode(&afterDenyResp)
	if afterDenyResp.Access != "ALLOWED" || afterDenyResp.RemainingSeconds < 6200 {
		t.Fatalf("Expected device to keep remaining time after denied extension, got %+v", afterDenyResp)
	}

	// 17. Parent removes / de-registers the child; child disappears from GET /api/children
	// and paired device receives 410 Gone (DEREGISTERED) on access-status
	req = httptest.NewRequest("DELETE", "/api/children/"+childID, nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Delete child failed: %d, %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/children", nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var afterDelChildren []models.ChildProfile
	json.NewDecoder(w.Body).Decode(&afterDelChildren)
	if len(afterDelChildren) != 0 {
		t.Fatalf("Expected 0 children after deletion, got %+v", afterDelChildren)
	}

	req = httptest.NewRequest("GET", "/api/devices/access-status", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("Expected 410 Gone for deregistered child's device, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestWindowsAutoUpdateEndpoints(t *testing.T) {
	tmpDB := "test_updates_" + time.Now().Format("150405.000") + ".db"
	database, err := db.InitDB(tmpDB)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer func() {
		database.Close()
		os.Remove(tmpDB)
		os.Remove(tmpDB + "-wal")
		os.Remove(tmpDB + "-shm")
	}()

	dlDir, err := os.MkdirTemp("", "screentime_dl_*")
	if err != nil {
		t.Fatalf("Failed to create temp downloads dir: %v", err)
	}
	defer os.RemoveAll(dlDir)

	server := api.NewServer(database, "", dlDir)

	// Before files exist -> 404
	req := httptest.NewRequest("GET", "/api/updates/windows", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 when no update package exists, got %d", w.Code)
	}

	// Write dummy installer and manifest
	dummyBytes := []byte("MZ-dummy-installer-payload")
	os.WriteFile(dlDir+"/ScreenTimeSetup.exe", dummyBytes, 0644)
	manifestJSON := `{"version":"1.2.3","sha256":"abc123def456","url":"/downloads/ScreenTimeSetup.exe","size_bytes":26}`
	os.WriteFile(dlDir+"/windows-latest.json", []byte(manifestJSON), 0644)

	// GET /api/updates/windows -> 200 OK with manifest
	req = httptest.NewRequest("GET", "/api/updates/windows", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for windows update manifest, got %d: %s", w.Code, w.Body.String())
	}
	var gotManifest map[string]interface{}
	json.NewDecoder(w.Body).Decode(&gotManifest)
	if gotManifest["version"] != "1.2.3" {
		t.Fatalf("Expected version 1.2.3, got %v", gotManifest["version"])
	}

	// GET /downloads/ScreenTimeSetup.exe -> 200 OK with binary payload
	req = httptest.NewRequest("GET", "/downloads/ScreenTimeSetup.exe", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), dummyBytes) {
		t.Fatalf("Expected 200 OK with exact installer bytes, got %d (%d bytes)", w.Code, w.Body.Len())
	}
}

func TestPushSubscriptionRotationAndCleanup(t *testing.T) {
	server, database, cleanup := setupTestServerWithDB(t)
	defer cleanup()

	// 1. Register parent, create child, pair device
	regPayload := `{"family_name":"The Millers","display_name":"Dad","email":"dad@example.com","password":"secretpassword123"}`
	req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewBufferString(regPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Register failed: %d %s", w.Code, w.Body.String())
	}
	var authResp models.AuthResponse
	json.NewDecoder(w.Body).Decode(&authResp)
	parentToken := authResp.Token

	req = httptest.NewRequest("POST", "/api/children", bytes.NewBufferString(`{"display_name":"Lucas","emergency_contact":""}`))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var childResp map[string]string
	json.NewDecoder(w.Body).Decode(&childResp)
	childID := childResp["id"]

	req = httptest.NewRequest("POST", "/api/children/"+childID+"/pairing-code", nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var codeResp models.GeneratePairingCodeResponse
	json.NewDecoder(w.Body).Decode(&codeResp)

	pairBody, _ := json.Marshal(models.PairDeviceRequest{
		PairingCode: codeResp.PairingCode,
		Platform:    "WINDOWS",
		DeviceName:  "Lucas PC",
	})
	req = httptest.NewRequest("POST", "/api/devices/pair", bytes.NewReader(pairBody))
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var pairResp models.PairDeviceResponse
	json.NewDecoder(w.Body).Decode(&pairResp)
	deviceToken := pairResp.AuthToken

	// 2. Mock push service that records headers and returns 403 Forbidden
	type pushCapture struct {
		urgency string
		ttl     string
		authHdr string
	}
	capturedCh := make(chan pushCapture, 1)
	mockPush := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case capturedCh <- pushCapture{
			urgency: r.Header.Get("Urgency"),
			ttl:     r.Header.Get("TTL"),
			authHdr: r.Header.Get("Authorization"),
		}:
		default:
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("VAPID credentials mismatch"))
	}))
	defer mockPush.Close()

	// Generate valid P-256 ECDH public key & 16-byte auth secret for webpush encryption
	_, x, y, err := elliptic.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate P256 key: %v", err)
	}
	p256dh := base64.StdEncoding.EncodeToString(elliptic.Marshal(elliptic.P256(), x, y))
	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("Failed to generate auth bytes: %v", err)
	}
	authSecret := base64.StdEncoding.EncodeToString(authBytes)

	// 3. Save initial subscription, then rotate with old_endpoint
	oldEndpoint := mockPush.URL + "/old-sub"
	newEndpoint := mockPush.URL + "/new-sub"

	sub1, _ := json.Marshal(map[string]string{
		"endpoint": oldEndpoint,
		"p256dh":   p256dh,
		"auth":     authSecret,
	})
	req = httptest.NewRequest("POST", "/api/push/subscribe", bytes.NewReader(sub1))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Initial subscribe failed: %d %s", w.Code, w.Body.String())
	}

	sub2, _ := json.Marshal(map[string]string{
		"endpoint":     newEndpoint,
		"p256dh":       p256dh,
		"auth":         authSecret,
		"old_endpoint": oldEndpoint,
	})
	req = httptest.NewRequest("POST", "/api/push/subscribe", bytes.NewReader(sub2))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Rotate subscribe failed: %d %s", w.Code, w.Body.String())
	}

	var count int
	if err := database.QueryRow("SELECT COUNT(1) FROM push_subscriptions WHERE endpoint = ?", oldEndpoint).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Expected old endpoint to be deleted on rotation, count=%d err=%v", count, err)
	}
	if err := database.QueryRow("SELECT COUNT(1) FROM push_subscriptions WHERE endpoint = ?", newEndpoint).Scan(&count); err != nil || count != 1 {
		t.Fatalf("Expected new endpoint to be present, count=%d err=%v", count, err)
	}

	// 4. Trigger push via POST /api/requests and verify Urgency, TTL, VAPID sub, and 403 cleanup
	req = httptest.NewRequest("POST", "/api/requests", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Create screen time request failed: %d %s", w.Code, w.Body.String())
	}

	var cap pushCapture
	select {
	case cap = <-capturedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for push request to hit mock push server")
	}

	if cap.urgency != "high" {
		t.Fatalf("Expected Urgency=high, got %q", cap.urgency)
	}
	if cap.ttl != "1800" {
		t.Fatalf("Expected TTL=1800, got %q", cap.ttl)
	}

	// Extract JWT from "vapid t=<jwt>, k=<pub>" and verify "sub" claim
	jwtStart := strings.Index(cap.authHdr, "vapid t=")
	commaIdx := strings.Index(cap.authHdr, ",")
	if jwtStart == -1 || commaIdx == -1 {
		t.Fatalf("Unexpected VAPID Authorization header: %q", cap.authHdr)
	}
	jwtStr := cap.authHdr[jwtStart+len("vapid t=") : commaIdx]
	parts := strings.Split(jwtStr, ".")
	if len(parts) != 3 {
		t.Fatalf("Invalid JWT in VAPID header: %q", jwtStr)
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("Failed to decode VAPID JWT payload: %v", err)
	}
	var jwtClaims map[string]interface{}
	if err := json.Unmarshal(claimsBytes, &jwtClaims); err != nil {
		t.Fatalf("Failed to parse VAPID JWT claims: %v", err)
	}
	if jwtClaims["sub"] != "mailto:admin@codeka.com" {
		t.Fatalf("Expected VAPID sub=mailto:admin@codeka.com, got %v", jwtClaims["sub"])
	}

	// Wait briefly for the async goroutine to delete the 403 subscription from SQLite
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		database.QueryRow("SELECT COUNT(1) FROM push_subscriptions WHERE endpoint = ?", newEndpoint).Scan(&count)
		if count == 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if count != 0 {
		t.Fatalf("Expected 403 Forbidden response to delete stale subscription, remaining count=%d", count)
	}
}

func TestStaticManifestAndNoCacheHeaders(t *testing.T) {
	tmpDB := "test_static_" + time.Now().Format("150405.000") + ".db"
	database, err := db.InitDB(tmpDB)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	defer func() {
		database.Close()
		os.Remove(tmpDB)
		os.Remove(tmpDB + "-wal")
		os.Remove(tmpDB + "-shm")
	}()

	staticDir := t.TempDir()
	if err := os.WriteFile(staticDir+"/index.html", []byte("<html><body>ok</body></html>"), 0644); err != nil {
		t.Fatalf("Write index.html: %v", err)
	}
	if err := os.WriteFile(staticDir+"/manifest.webmanifest", []byte(`{"name":"Screen Time Admin"}`), 0644); err != nil {
		t.Fatalf("Write manifest.webmanifest: %v", err)
	}
	if err := os.WriteFile(staticDir+"/manifest.json", []byte(`{"name":"Screen Time Admin"}`), 0644); err != nil {
		t.Fatalf("Write manifest.json: %v", err)
	}
	if err := os.WriteFile(staticDir+"/sw.js", []byte("self.addEventListener('fetch', () => {});"), 0644); err != nil {
		t.Fatalf("Write sw.js: %v", err)
	}

	server := api.NewServer(database, staticDir)

	// 1. manifest.webmanifest returns application/manifest+json and no-cache
	req := httptest.NewRequest("GET", "/manifest.webmanifest", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for manifest.webmanifest, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/manifest+json") {
		t.Fatalf("Expected Content-Type application/manifest+json, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Fatalf("Expected no-cache Cache-Control for manifest.webmanifest, got %q", cc)
	}

	// 2. sw.js returns no-cache
	req = httptest.NewRequest("GET", "/sw.js", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for sw.js, got %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Fatalf("Expected no-cache Cache-Control for sw.js, got %q", cc)
	}

	// 3. SPA route fallback serves index.html with no-cache
	req = httptest.NewRequest("GET", "/dashboard", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for /dashboard fallback, got %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Fatalf("Expected no-cache Cache-Control for /dashboard SPA fallback, got %q", cc)
	}
}

