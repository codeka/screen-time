package schedule

import (
	"database/sql"
	"fmt"
	"screentime-server/models"
	"strconv"
	"strings"
	"time"
)

type EvaluationResult struct {
	IsUnrestricted    bool
	ValidUntil        *time.Time
	RemainingSeconds  int64
	NextScheduleStart *time.Time
}

// EvaluateChildSchedule checks whether childID currently falls within an active unrestricted schedule rule.
func EvaluateChildSchedule(database *sql.DB, childID string, now time.Time) (*EvaluationResult, error) {
	rows, err := database.Query(`
		SELECT id, child_id, day_of_week, start_time, end_time, timezone, enabled
		FROM schedule_rules
		WHERE child_id = ? AND enabled = 1
	`, childID)
	if err != nil {
		return nil, fmt.Errorf("failed to query schedule rules: %w", err)
	}
	defer rows.Close()

	var rules []models.ScheduleRule
	for rows.Next() {
		var r models.ScheduleRule
		if err := rows.Scan(&r.ID, &r.ChildID, &r.DayOfWeek, &r.StartTime, &r.EndTime, &r.Timezone, &r.Enabled); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}

	result := &EvaluationResult{
		IsUnrestricted: false,
	}

	for _, rule := range rules {
		loc := time.Local
		if rule.Timezone != "" && rule.Timezone != "Local" {
			if l, err := time.LoadLocation(rule.Timezone); err == nil {
				loc = l
			}
		}

		nowInLoc := now.In(loc)
		currentWeekday := int(nowInLoc.Weekday())

		if currentWeekday != rule.DayOfWeek {
			continue
		}

		startHour, startMin, err1 := parseTimeHM(rule.StartTime)
		endHour, endMin, err2 := parseTimeHM(rule.EndTime)
		if err1 != nil || err2 != nil {
			continue
		}

		ruleStart := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), startHour, startMin, 0, 0, loc)
		ruleEnd := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), endHour, endMin, 0, 0, loc)

		// If end time is before start time (e.g. cross-midnight rule 22:00 -> 01:00)
		if ruleEnd.Before(ruleStart) {
			ruleEnd = ruleEnd.Add(24 * time.Hour)
		}

		if (nowInLoc.Equal(ruleStart) || nowInLoc.After(ruleStart)) && nowInLoc.Before(ruleEnd) {
			result.IsUnrestricted = true
			untilUTC := ruleEnd.UTC()
			result.ValidUntil = &untilUTC
			result.RemainingSeconds = int64(ruleEnd.Sub(nowInLoc).Seconds())
			return result, nil
		}
	}

	return result, nil
}

func parseTimeHM(val string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(val), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time format: %s", val)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return h, m, nil
}

