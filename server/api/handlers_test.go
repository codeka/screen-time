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
	"screentime-server/db"
	"screentime-server/models"
)

func setupTestServer(t *testing.T) (*api.Server, func()) {
	tmpFile := "test_screentime_" + time.Now().Format("150405.000") + ".db"
	database, err := db.InitDB(tmpFile)
	if err != nil {
		t.Fatalf("Failed to init test db: %v", err)
	}

	server := api.NewServer(database)

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
}

