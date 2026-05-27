package store

import (
	"database/sql"
	"math"
)

func (s *Store) Stats(slowestLimit int, commandPattern string) (*Stats, error) {
	var total, completed, successful, failed, running, activeDays sql.NullInt64
	var firstStarted, lastStarted sql.NullString
	var avgDuration, minDuration, maxDuration sql.NullFloat64

	where, args := commandFilter("WHERE", commandPattern, StatusSuccess, StatusFailed, StatusRunning)
	err := s.db.QueryRow(`SELECT
COUNT(*),
SUM(CASE WHEN finished_at IS NOT NULL THEN 1 ELSE 0 END),
SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
COUNT(DISTINCT substr(started_at, 1, 10)),
MIN(started_at),
MAX(started_at),
AVG(duration_ms),
MIN(duration_ms),
MAX(duration_ms)
FROM runs`+where, args...).Scan(
		&total,
		&completed,
		&successful,
		&failed,
		&running,
		&activeDays,
		&firstStarted,
		&lastStarted,
		&avgDuration,
		&minDuration,
		&maxDuration,
	)
	if err != nil {
		return nil, err
	}
	stats := &Stats{
		TotalRuns:      total.Int64,
		CompletedRuns:  completed.Int64,
		SuccessfulRuns: successful.Int64,
		FailedRuns:     failed.Int64,
		RunningRuns:    running.Int64,
		ActiveDays:     activeDays.Int64,
	}
	if firstStarted.Valid {
		if t, err := parseTime(firstStarted.String); err == nil {
			stats.FirstStartedAt = &t
		}
	}
	if lastStarted.Valid {
		if t, err := parseTime(lastStarted.String); err == nil {
			stats.LastStartedAt = &t
		}
	}
	if avgDuration.Valid {
		v := int64(math.Round(avgDuration.Float64))
		stats.AvgDurationMS = &v
	}
	if minDuration.Valid {
		v := int64(math.Round(minDuration.Float64))
		stats.MinDurationMS = &v
	}
	if maxDuration.Valid {
		v := int64(math.Round(maxDuration.Float64))
		stats.MaxDurationMS = &v
	}
	if slowestLimit <= 0 {
		return stats, nil
	}
	slowWhere, slowArgs := commandFilter("AND", commandPattern)
	slowArgs = append(slowArgs, slowestLimit)
	rows, err := s.db.Query(`SELECT id,status,mode,command,argv_json,cwd,started_at,duration_ms
FROM runs
WHERE duration_ms IS NOT NULL`+slowWhere+`
ORDER BY duration_ms DESC, id DESC
LIMIT ?`, slowArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var run SlowRun
		var started string
		if err := rows.Scan(&run.ID, &run.Status, &run.Mode, &run.Command, &run.ArgvJSON, &run.CWD, &started, &run.DurationMS); err != nil {
			return nil, err
		}
		run.StartedAt, _ = parseTime(started)
		stats.SlowestRuns = append(stats.SlowestRuns, run)
	}
	return stats, rows.Err()
}

// commandFilter builds a SQL fragment for command LIKE filtering.
// keyword should be "WHERE" (no prior conditions) or "AND" (existing WHERE).
// fixedArgs are prepended before the LIKE arg. When pattern is empty the
// clause is empty and only fixedArgs are returned.
func commandFilter(keyword, pattern string, fixedArgs ...interface{}) (string, []interface{}) {
	args := make([]interface{}, 0, len(fixedArgs)+1)
	args = append(args, fixedArgs...)
	if pattern == "" {
		return "", args
	}
	args = append(args, "%"+pattern+"%")
	return " " + keyword + " command LIKE ?", args
}
