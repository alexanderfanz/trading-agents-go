package checkpoint

import (
	"context"
	"os"
	"time"
)

// CleanupWorker handles cron-style database prunings and vacuum schedules.
type CleanupWorker struct {
	mgr           *SQLConnectionManager
	interval      time.Duration
	maxSizeBytes  int64
	retentionDays int
	stopChan      chan struct{}
}

// NewCleanupWorker initializes the background pruning process.
func NewCleanupWorker(mgr *SQLConnectionManager, interval time.Duration, maxSizeBytes int64, retentionDays int) *CleanupWorker {
	return &CleanupWorker{
		mgr:           mgr,
		interval:      interval,
		maxSizeBytes:  maxSizeBytes,
		retentionDays: retentionDays,
		stopChan:      make(chan struct{}),
	}
}

// Start spawns the background routine.
func (w *CleanupWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				w.executePruneAndVacuum(ctx)
			case <-w.stopChan:
				ticker.Stop()
				return
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop terminates the scheduler loops.
func (w *CleanupWorker) Stop() {
	close(w.stopChan)
}

func (w *CleanupWorker) executePruneAndVacuum(ctx context.Context) {
	// 1. Execute Cron deletion of expired checkpoint rows
	pruneQuery := `DELETE FROM checkpoints WHERE updated_at < datetime('now', printf('-%d days', ?));`
	res, err := w.mgr.writeDB.ExecContext(ctx, pruneQuery, w.retentionDays)
	if err == nil {
		if _, err := res.RowsAffected(); err == nil {
			// Row deletion succeeded, logging is handled by system logger
		}
	}

	// 2. Measure actual database size on disk
	fi, err := os.Stat(w.mgr.dbPath)
	if err != nil {
		return
	}

	// If database size exceeds the threshold, trigger a non-blocking VACUUM
	if fi.Size() > w.maxSizeBytes {
		w.runVacuum(ctx)
	}
}

func (w *CleanupWorker) runVacuum(ctx context.Context) {
	// VACUUM cannot be run inside a transaction. We must run it on the write connection
	// when no other active statements are processing.
	_, _ = w.mgr.writeDB.ExecContext(ctx, "VACUUM;")
}
