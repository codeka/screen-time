package schedule_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"screentime-server/db"
	"screentime-server/schedule"
)

func setupDB(t *testing.T) (*sql.DB, func()) {
	tmp := fmt.Sprintf("test_sched_%d.db", time.Now().UnixNano())
	database, err := db.InitDB(tmp)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}
	return database, func() {
		database.Close()
		os.Remove(tmp)
		os.Remove(tmp + "-wal")
		os.Remove(tmp + "-shm")
	}
}

func TestEvaluateChildSchedule(t *testing.T) {
	database, cleanup := setupDB(t)
	defer cleanup()

	// Seed family & child
	database.Exec("INSERT INTO families (id, name) VALUES ('f1', 'Test Family')")
	database.Exec("INSERT INTO child_profiles (id, family_id, display_name) VALUES ('c1', 'f1', 'Lucas')")

	now := time.Now()
	todayWeekday := int(now.Weekday())

	// Rule 1: Active right now (started 30 mins ago, ends in 30 mins)
	startStr := now.Add(-30 * time.Minute).Format("15:04")
	endStr := now.Add(30 * time.Minute).Format("15:04")

	_, err := database.Exec(`
		INSERT INTO schedule_rules (id, child_id, day_of_week, start_time, end_time, timezone, enabled)
		VALUES ('r1', 'c1', ?, ?, ?, 'Local', 1)
	`, todayWeekday, startStr, endStr)
	if err != nil {
		t.Fatalf("Failed to insert rule: %v", err)
	}

	res, err := schedule.EvaluateChildSchedule(database, "c1", now)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if !res.IsUnrestricted {
		t.Errorf("Expected IsUnrestricted to be true")
	}
	if res.RemainingSeconds <= 0 || res.RemainingSeconds > 1800 {
		t.Errorf("Unexpected remaining seconds: %d", res.RemainingSeconds)
	}

	// Rule 2: Disabled rule should not grant access
	database.Exec("UPDATE schedule_rules SET enabled = 0 WHERE id = 'r1'")
	resDisabled, err := schedule.EvaluateChildSchedule(database, "c1", now)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if resDisabled.IsUnrestricted {
		t.Errorf("Expected IsUnrestricted to be false when rule is disabled")
	}
}

