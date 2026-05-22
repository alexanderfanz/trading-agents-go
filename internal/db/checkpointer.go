package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver (no CGO required)
)

type Checkpointer struct {
	mu       sync.Mutex
	cacheDir string
	dbMap    map[string]*sql.DB
}

func NewCheckpointer(cacheDir string) *Checkpointer {
	_ = os.MkdirAll(cacheDir, 0755)
	return &Checkpointer{
		cacheDir: cacheDir,
		dbMap:    make(map[string]*sql.DB),
	}
}

func (c *Checkpointer) getDB(companyName string) (*sql.DB, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if db, ok := c.dbMap[companyName]; ok {
		return db, nil
	}

	dbPath := filepath.Join(c.cacheDir, fmt.Sprintf("%s_checkpoint.db", companyName))
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite checkpoint db: %w", err)
	}

	// Create tables if not exist
	query := `
	CREATE TABLE IF NOT EXISTS checkpoints (
		thread_id TEXT PRIMARY KEY,
		step_index INTEGER,
		state_json TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(query); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create checkpoints table: %w", err)
	}

	c.dbMap[companyName] = db
	return db, nil
}

func (c *Checkpointer) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, db := range c.dbMap {
		_ = db.Close()
	}
	c.dbMap = make(map[string]*sql.DB)
}

func makeThreadID(companyName, tradeDate string) string {
	return fmt.Sprintf("%s_%s", companyName, tradeDate)
}

// Save stores the checkpoint step and state JSON.
func (c *Checkpointer) Save(companyName, tradeDate string, stepIndex int, stateJSON string) error {
	db, err := c.getDB(companyName)
	if err != nil {
		return err
	}

	threadID := makeThreadID(companyName, tradeDate)
	query := `
	INSERT INTO checkpoints (thread_id, step_index, state_json, updated_at)
	VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(thread_id) DO UPDATE SET
		step_index = excluded.step_index,
		state_json = excluded.state_json,
		updated_at = excluded.updated_at;`

	if _, err := db.Exec(query, threadID, stepIndex, stateJSON); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

// Load returns the step index and state JSON. If not found, returns -1, "", nil.
func (c *Checkpointer) Load(companyName, tradeDate string) (int, string, error) {
	db, err := c.getDB(companyName)
	if err != nil {
		return -1, "", err
	}

	threadID := makeThreadID(companyName, tradeDate)
	query := `SELECT step_index, state_json FROM checkpoints WHERE thread_id = ?;`

	var stepIndex int
	var stateJSON string
	err = db.QueryRow(query, threadID).Scan(&stepIndex, &stateJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return -1, "", nil
		}
		return -1, "", fmt.Errorf("failed to query checkpoint: %w", err)
	}

	return stepIndex, stateJSON, nil
}

// Clear deletes the checkpoint row.
func (c *Checkpointer) Clear(companyName, tradeDate string) error {
	db, err := c.getDB(companyName)
	if err != nil {
		return err
	}

	threadID := makeThreadID(companyName, tradeDate)
	query := `DELETE FROM checkpoints WHERE thread_id = ?;`

	if _, err := db.Exec(query, threadID); err != nil {
		return fmt.Errorf("failed to clear checkpoint: %w", err)
	}

	return nil
}
