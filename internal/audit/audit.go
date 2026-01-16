package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Direction string

const (
	DirectionUpstreamToDownstream Direction = "upstream_to_downstream"
	DirectionDownstreamToUpstream Direction = "downstream_to_upstream"
)

type Record struct {
	Timestamp time.Time
	Direction Direction
	SessionID string
	Method    string
	IsRequest bool
	IsNotify  bool
	ID        json.RawMessage
	Raw       json.RawMessage

	// Optional extracted user/agent text, if any.
	UserText  string
	AgentText string
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Keep it simple; callers can tune via DSN if needed.
	if err := initSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func initSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts_unix_ms INTEGER NOT NULL,
  direction TEXT NOT NULL,
  session_id TEXT,
  method TEXT,
  is_request INTEGER NOT NULL,
  is_notify INTEGER NOT NULL,
  rpc_id TEXT,
  raw_json TEXT NOT NULL,
  user_text TEXT,
  agent_text TEXT
);
CREATE INDEX IF NOT EXISTS idx_audit_events_ts ON audit_events(ts_unix_ms);
CREATE INDEX IF NOT EXISTS idx_audit_events_session ON audit_events(session_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_method ON audit_events(method);
`)
	return err
}

func (s *Store) Write(ctx context.Context, r Record) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("audit store not initialized")
	}
	rawStr := string(r.Raw)
	rpcID := ""
	if len(r.ID) > 0 {
		rpcID = string(r.ID)
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO audit_events(
  ts_unix_ms, direction, session_id, method, is_request, is_notify, rpc_id, raw_json, user_text, agent_text
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, r.Timestamp.UnixMilli(), string(r.Direction), nullIfEmpty(r.SessionID), nullIfEmpty(r.Method), boolInt(r.IsRequest), boolInt(r.IsNotify), nullIfEmpty(rpcID), rawStr, nullIfEmpty(r.UserText), nullIfEmpty(r.AgentText))
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
