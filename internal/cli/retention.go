package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

const (
	sessionRetentionInterval = 24 * time.Hour
	sessionRetentionFilename = "session-retention.timestamp"
)

// expireAutoSessionsIfDue deletes expired automatic sessions at most once a
// day, when a shell starts. Errors are ignored: cleanup must never stop a
// terminal from opening.
func (a *app) expireAutoSessionsIfDue(cmd *cobra.Command) {
	dataDir := a.cfg.Storage.DataDir
	if a.cfg.AutoSessionRetentionDuration() == 0 || !sidecarDue(dataDir, sessionRetentionFilename, sessionRetentionInterval) {
		return
	}
	touchSidecar(dataDir, sessionRetentionFilename)
	_, _, _ = a.expireAutoSessions(time.Now(), false)
}

// expireAutoSessions finds automatic sessions that ended longer ago than
// shell.auto_session_retention and have no label, notes, or pin. Unless
// dryRun, it deletes them with their runs and logs. It returns the expired
// sessions and their runs.
func (a *app) expireAutoSessions(now time.Time, dryRun bool) ([]store.Session, []store.Run, error) {
	retention := a.cfg.AutoSessionRetentionDuration()
	if retention == 0 {
		return nil, nil, nil
	}
	sessions, err := a.st.ExpiredAutoSessions(now.Add(-retention))
	if err != nil {
		return nil, nil, err
	}
	var runs []store.Run
	for _, sess := range sessions {
		var sessRuns []store.Run
		if dryRun {
			sessRuns, err = a.st.SessionRuns(sess.ID)
		} else {
			sessRuns, err = a.st.DeleteSession(sess.ID)
		}
		if err != nil {
			return nil, nil, err
		}
		if !dryRun {
			for _, r := range sessRuns {
				removeIfSet(r.StdoutPath)
				removeIfSet(r.StderrPath)
				removeIfSet(r.TerminalOutputPath)
			}
		}
		runs = append(runs, sessRuns...)
	}
	if !dryRun && len(sessions) > 0 {
		_, _ = a.st.PruneOrphanedEnvironments()
	}
	return sessions, runs, nil
}
