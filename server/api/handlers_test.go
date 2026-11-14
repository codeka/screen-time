package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
}

