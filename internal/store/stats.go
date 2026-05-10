package store

import (
	"database/sql"
	"math"
)

func (s *Store) Stats(slowestLimit int) (*Stats, error) {
	var total, completed, successful, failed, running, activeDays sql.NullInt64
	var firstStarted, lastStarted sql.NullString
	var avgDuration sql.NullFloat64
	err := s.db.QueryRow(`SELECT
COUNT(*),
SUM(CASE WHEN finished_at IS NOT NULL THEN 1 ELSE 0 END),
SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
COUNT(DISTINCT substr(started_at, 1, 10)),
MIN(started_at),
MAX(started_at),
AVG(duration_ms)
FROM runs`, StatusSuccess, StatusFailed, StatusRunning).Scan(
		&total,
		&completed,
		&successful,
		&failed,
		&running,
		&activeDays,
		&firstStarted,
		&lastStarted,
		&avgDuration,
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
	if slowestLimit <= 0 {
		return stats, nil
	}
	rows, err := s.db.Query(`SELECT id,status,mode,command,argv_json,cwd,started_at,duration_ms
FROM runs
WHERE duration_ms IS NOT NULL
ORDER BY duration_ms DESC, id DESC
LIMIT ?`, slowestLimit)
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
