package checkpoint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestManager(t *testing.T) (*SQLConnectionManager, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "vacuum_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	dbPath := filepath.Join(tempDir, "checkpoints.db")
	mgr, err := NewSQLConnectionManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create connection manager: %v", err)
	}
	t.Cleanup(func() {
		_ = mgr.Close()
	})

	return mgr, dbPath
}

func insertCheckpointRow(t *testing.T, mgr *SQLConnectionManager, id, ticker, tradeDate, updatedAtExpr string) {
	t.Helper()

	query := `
	INSERT INTO checkpoints (id, ticker, trade_date, step_index, state_data, checksum, version, updated_at)
	VALUES (?, ?, ?, 0, ?, 'checksum', '1.0.0', ` + updatedAtExpr + `);` // #nosec G202 -- test-only SQL datetime expressions

	_, err := mgr.writeDB.Exec(query, id, ticker, tradeDate, []byte("test-payload"))
	if err != nil {
		t.Fatalf("failed to insert checkpoint row %s: %v", id, err)
	}
}

func countCheckpointRows(t *testing.T, mgr *SQLConnectionManager) int {
	t.Helper()

	var count int
	err := mgr.readDB.QueryRow(`SELECT COUNT(*) FROM checkpoints;`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count checkpoint rows: %v", err)
	}
	return count
}

func TestExecutePruneAndVacuum_PrunesOldCheckpoints(t *testing.T) {
	mgr, _ := setupTestManager(t)
	ctx := context.Background()

	insertCheckpointRow(t, mgr, "OLD:AAPL", "AAPL", "2026-01-01", "datetime('now', '-10 days')")
	insertCheckpointRow(t, mgr, "NEW:MSFT", "MSFT", "2026-05-22", "CURRENT_TIMESTAMP")

	before := countCheckpointRows(t, mgr)
	if before != 2 {
		t.Fatalf("expected 2 seeded rows, got %d", before)
	}

	worker := NewCleanupWorker(mgr, time.Minute, 1<<20, 7)
	worker.executePruneAndVacuum(ctx)

	after := countCheckpointRows(t, mgr)
	if after >= before {
		t.Fatalf("expected prune to delete old rows, before=%d after=%d", before, after)
	}
	if after != 1 {
		t.Fatalf("expected exactly one recent row to remain, got %d", after)
	}

	var remainingID string
	err := mgr.readDB.QueryRowContext(ctx, "SELECT id FROM checkpoints LIMIT 1").Scan(&remainingID)
	if err != nil {
		t.Fatalf("failed to query remaining row: %v", err)
	}
	if remainingID != "NEW:MSFT" {
		t.Errorf("expected remaining row to be 'NEW:MSFT', got %q", remainingID)
	}
}

func TestExecutePruneAndVacuum_TriggersVacuumWhenOverSizeThreshold(t *testing.T) {
	mgr, dbPath := setupTestManager(t)
	ctx := context.Background()

	largePayload := make([]byte, 256*1024)
	for i := range largePayload {
		largePayload[i] = 'x'
	}

	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("BLOB:%d", i)
		_, err := mgr.writeDB.Exec(`
			INSERT INTO checkpoints (id, ticker, trade_date, step_index, state_data, checksum, version, updated_at)
			VALUES (?, ?, ?, 0, ?, 'checksum', '1.0.0', CURRENT_TIMESTAMP);`,
			id, "AAPL", "2026-05-22", largePayload,
		)
		if err != nil {
			t.Fatalf("failed to insert large checkpoint row: %v", err)
		}
	}

	_, err := mgr.writeDB.Exec(`DELETE FROM checkpoints;`)
	if err != nil {
		t.Fatalf("failed to delete checkpoint rows: %v", err)
	}

	fi, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("failed to stat database before vacuum: %v", err)
	}
	sizeBeforeVacuum := fi.Size()
	if sizeBeforeVacuum <= 1024 {
		t.Fatalf("expected bloated database file before vacuum, got %d bytes", sizeBeforeVacuum)
	}

	worker := NewCleanupWorker(mgr, time.Minute, 1024, 0)
	worker.executePruneAndVacuum(ctx)

	fi, err = os.Stat(dbPath)
	if err != nil {
		t.Fatalf("failed to stat database after vacuum: %v", err)
	}
	if fi.Size() >= sizeBeforeVacuum {
		t.Fatalf("expected VACUUM to shrink database file, before=%d after=%d", sizeBeforeVacuum, fi.Size())
	}
}

func TestCleanupWorker_StartStop(t *testing.T) {
	t.Run("context cancel exits cleanly", func(t *testing.T) {
		mgr, _ := setupTestManager(t)

		ctx, cancel := context.WithCancel(context.Background())
		worker := NewCleanupWorker(mgr, 5*time.Millisecond, 1<<20, 7)
		worker.Start(ctx)

		time.Sleep(25 * time.Millisecond)
		cancel()

		time.Sleep(25 * time.Millisecond)
	})

	t.Run("stop exits cleanly", func(t *testing.T) {
		mgr, _ := setupTestManager(t)

		worker := NewCleanupWorker(mgr, 5*time.Millisecond, 1<<20, 7)
		worker.Start(context.Background())

		time.Sleep(25 * time.Millisecond)
		worker.Stop()

		time.Sleep(25 * time.Millisecond)
	})
}
