package misc

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// configDB is the singleton SQLite database for configuration.
var (
	configDB   *sql.DB
	configOnce sync.Once
)

// configDefault defines a single default config entry.
type configDefault struct {
	Section  string
	Key      string
	Value    string
	Required bool   // if true, value must be set by user (empty default)
	Label    string // user-friendly display name (used in error messages)
}

// allDefaults lists every known config key with its default value.
// Required entries have empty Value and Required=true — the system will
// insert them with empty value on first run so the user knows to fill them.
var allDefaults = []configDefault{
	// [misc]
	{Section: "misc", Key: "MessageMaximum", Value: "10240"},
	{Section: "misc", Key: "MaxTryCount", Value: "5"},
	{Section: "misc", Key: "PROJECT_TASK_MAX_CONCURRENCY", Value: "2"},
	{Section: "misc", Key: "DATA_DIR", Value: "./data"},
	{Section: "misc", Key: "GITHUB_COPILOT_MCP_AUTHORIZATION", Value: ""},
	{Section: "misc", Key: "FeiShuAPI", Value: ""},
	{Section: "misc", Key: "DEBUG", Value: "false"},

	// [main_setting] — global LLM defaults
	{Section: "main_setting", Key: "BASE_URL", Value: "", Required: true, Label: "API 地址 (BASE_URL)"},
	{Section: "main_setting", Key: "OPENAI_API_KEY", Value: "", Required: true, Label: "API 密钥 (OPENAI_API_KEY)"},
	{Section: "main_setting", Key: "MODEL", Value: "", Required: true, Label: "模型名称 (MODEL)"},
	{Section: "main_setting", Key: "MaxContext", Value: "100"},
	{Section: "main_setting", Key: "MaxRequest", Value: "5"},
	{Section: "main_setting", Key: "USER_AGENT", Value: "AIxVuln"},
	{Section: "main_setting", Key: "STREAM", Value: "false"},
	{Section: "main_setting", Key: "API_MODE", Value: "chat"},
}

// initConfigDB opens (or creates) the SQLite config database and inserts
// default rows for any key that does not yet exist.
func initConfigDB() {
	configOnce.Do(func() {
		// Determine DB path: data/AIxVuln.db
		dataDir := "./data"
		// If DATA_DIR was set via environment, honour it.
		if env := os.Getenv("AIXVULN_DATA_DIR"); env != "" {
			dataDir = env
		}
		absDir, _ := filepath.Abs(dataDir)
		if err := os.MkdirAll(absDir, 0755); err != nil {
			panic(fmt.Sprintf("cannot create data dir %s: %v", absDir, err))
		}
		dbPath := filepath.Join(absDir, "AIxVuln.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			panic(fmt.Sprintf("cannot open config db %s: %v", dbPath, err))
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)

		// Create table.
		_, err = db.Exec(`
			PRAGMA journal_mode=WAL;
			CREATE TABLE IF NOT EXISTS config (
				section TEXT NOT NULL,
				key     TEXT NOT NULL,
				value   TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (section, key)
			);
		`)
		if err != nil {
			panic(fmt.Sprintf("cannot create config table: %v", err))
		}

		// Insert defaults for keys that don't exist yet.
		stmt, err := db.Prepare(`INSERT OR IGNORE INTO config (section, key, value) VALUES (?, ?, ?)`)
		if err != nil {
			panic(fmt.Sprintf("cannot prepare default insert: %v", err))
		}
		defer stmt.Close()
		for _, d := range allDefaults {
			_, _ = stmt.Exec(d.Section, d.Key, d.Value)
		}

		configDB = db
	})
}

// CheckRequiredConfig returns a list of missing required config entries.
// Each entry is formatted as "section.key".
// Returns nil if all required config is set.
func CheckRequiredConfig() []string {
	var missing []string
	for _, d := range allDefaults {
		if !d.Required {
			continue
		}
		val := strings.TrimSpace(dbGet(d.Section, d.Key))
		if val == "" {
			if d.Label != "" {
				missing = append(missing, d.Label)
			} else {
				missing = append(missing, d.Section+"."+d.Key)
			}
		}
	}
	return missing
}

// dbGet reads a single config value from SQLite.
// Returns empty string if not found.
func dbGet(section, key string) string {
	initConfigDB()
	var value string
	err := configDB.QueryRow(`SELECT value FROM config WHERE section = ? AND key = ?`, section, key).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

// dbSet writes a single config value to SQLite.
func dbSet(section, key, value string) error {
	initConfigDB()
	_, err := configDB.Exec(`INSERT INTO config (section, key, value) VALUES (?, ?, ?)
		ON CONFLICT(section, key) DO UPDATE SET value = excluded.value`, section, key, value)
	return err
}

// dbDelete removes a single config key from SQLite.
func dbDelete(section, key string) error {
	initConfigDB()
	_, err := configDB.Exec(`DELETE FROM config WHERE section = ? AND key = ?`, section, key)
	return err
}

// dbDeleteSection removes all keys in a section from SQLite.
func dbDeleteSection(section string) error {
	initConfigDB()
	_, err := configDB.Exec(`DELETE FROM config WHERE section = ?`, section)
	return err
}

// dbGetAll returns all config as map[section]map[key]value.
func dbGetAll() map[string]map[string]string {
	initConfigDB()
	rows, err := configDB.Query(`SELECT section, key, value FROM config ORDER BY section, key`)
	if err != nil {
		return make(map[string]map[string]string)
	}
	defer rows.Close()
	result := make(map[string]map[string]string)
	for rows.Next() {
		var section, key, value string
		if err := rows.Scan(&section, &key, &value); err != nil {
			continue
		}
		if result[section] == nil {
			result[section] = make(map[string]string)
		}
		result[section][key] = value
	}
	return result
}

// dbSetAll replaces all config with the provided data.
// It deletes all existing rows and inserts the new ones, then re-inserts
// any missing defaults.
func dbSetAll(data map[string]map[string]string) error {
	initConfigDB()
	tx, err := configDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear all.
	if _, err := tx.Exec(`DELETE FROM config`); err != nil {
		return err
	}

	// Insert new data.
	stmt, err := tx.Prepare(`INSERT INTO config (section, key, value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for section, kv := range data {
		for k, v := range kv {
			if _, err := stmt.Exec(section, k, v); err != nil {
				return err
			}
		}
	}

	// Re-insert defaults for any missing keys (INSERT OR IGNORE).
	stmtDef, err := tx.Prepare(`INSERT OR IGNORE INTO config (section, key, value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmtDef.Close()
	for _, d := range allDefaults {
		_, _ = stmtDef.Exec(d.Section, d.Key, d.Value)
	}

	return tx.Commit()
}
