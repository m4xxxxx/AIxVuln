package Web

import (
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type projectStateStore struct {
	mu sync.Mutex
	db *sql.DB
}

func newProjectStateStore(dataDir string) (*projectStateStore, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(absDir, "project_state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA temp_store=MEMORY;
		CREATE TABLE IF NOT EXISTS project_state (
			project_name TEXT PRIMARY KEY,
			payload      TEXT NOT NULL,
			updated_at   INTEGER NOT NULL
		);
	`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &projectStateStore{db: db}, nil
}

func (s *projectStateStore) SaveAll(projects []taskManager.ProjectInfo) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("project state store is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	upsertStmt, err := tx.Prepare(`
		INSERT INTO project_state(project_name, payload, updated_at)
		VALUES(?, ?, ?)
		ON CONFLICT(project_name) DO UPDATE SET
			payload=excluded.payload,
			updated_at=excluded.updated_at;
	`)
	if err != nil {
		return err
	}
	defer upsertStmt.Close()

	now := time.Now().Unix()
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		name := strings.TrimSpace(p.ProjectName)
		if name == "" {
			continue
		}
		js, err := json.Marshal(p)
		if err != nil {
			misc.Debug("projectStateStore.SaveAll: marshal failed for %s: %v", name, err)
			continue
		}
		if _, err := upsertStmt.Exec(name, string(js), now); err != nil {
			return err
		}
		names = append(names, name)
	}

	if len(names) == 0 {
		if _, err := tx.Exec(`DELETE FROM project_state`); err != nil {
			return err
		}
	} else {
		placeholders := make([]string, len(names))
		args := make([]interface{}, 0, len(names))
		for i, n := range names {
			placeholders[i] = "?"
			args = append(args, n)
		}
		q := `DELETE FROM project_state WHERE project_name NOT IN (` + strings.Join(placeholders, ",") + `)`
		if _, err := tx.Exec(q, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *projectStateStore) LoadAll() ([]taskManager.ProjectInfo, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("project state store is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT payload FROM project_state ORDER BY project_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]taskManager.ProjectInfo, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		var p taskManager.ProjectInfo
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			misc.Debug("projectStateStore.LoadAll: unmarshal payload failed: %v", err)
			continue
		}
		if strings.TrimSpace(p.ProjectName) == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *projectStateStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
