package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database with typed methods for cameras and events.
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cameras (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			name          TEXT NOT NULL,
			host          TEXT NOT NULL,
			username      TEXT NOT NULL DEFAULT '',
			password      TEXT NOT NULL DEFAULT '',
			rtsp_subtype  INTEGER NOT NULL DEFAULT 1,
			enabled       INTEGER NOT NULL DEFAULT 1,
			created_at    TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id   INTEGER NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
			code        TEXT NOT NULL,
			action      TEXT NOT NULL,
			channel_idx INTEGER NOT NULL DEFAULT 0,
			data_json   TEXT NOT NULL DEFAULT '',
			occurred_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS events_camera_time ON events(camera_id, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS events_time ON events(occurred_at DESC)`,
		`CREATE TABLE IF NOT EXISTS recordings (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id    INTEGER NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
			event_id     INTEGER REFERENCES events(id) ON DELETE SET NULL,
			started_at   TEXT NOT NULL,
			ended_at     TEXT NOT NULL,
			duration_ms  INTEGER NOT NULL,
			path         TEXT NOT NULL,
			size_bytes   INTEGER NOT NULL DEFAULT 0,
			created_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS recordings_camera_time ON recordings(camera_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS recordings_event ON recordings(event_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w (%s)", err, q)
		}
	}
	return nil
}

// Camera mirrors a row in the cameras table.
type Camera struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // never returned in JSON
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// Event mirrors a row in the events table.
type Event struct {
	ID         int64     `json:"id"`
	CameraID   int64     `json:"camera_id"`
	CameraName string    `json:"camera_name,omitempty"`
	Code       string    `json:"code"`
	Action     string    `json:"action"`
	ChannelIdx int       `json:"channel_idx"`
	DataJSON   string    `json:"data_json,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Recording mirrors a row in the recordings table. Path is server-local.
type Recording struct {
	ID         int64     `json:"id"`
	CameraID   int64     `json:"camera_id"`
	CameraName string    `json:"camera_name,omitempty"`
	EventID    int64     `json:"event_id,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMs int64     `json:"duration_ms"`
	Path       string    `json:"-"`
	SizeBytes  int64     `json:"size_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

var ErrNotFound = errors.New("not found")

func (s *Store) ListCameras(ctx context.Context) ([]Camera, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, host, username, password, enabled, created_at
		FROM cameras ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Camera
	for rows.Next() {
		c, err := scanCamera(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCamera(ctx context.Context, id int64) (Camera, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, host, username, password, enabled, created_at
		FROM cameras WHERE id = ?`, id)
	c, err := scanCamera(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Camera{}, ErrNotFound
	}
	return c, err
}

func (s *Store) CreateCamera(ctx context.Context, c Camera) (Camera, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO cameras (name, host, username, password, enabled)
		VALUES (?, ?, ?, ?, ?)`,
		c.Name, c.Host, c.Username, c.Password, boolToInt(c.Enabled))
	if err != nil {
		return Camera{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Camera{}, err
	}
	return s.GetCamera(ctx, id)
}

func (s *Store) UpdateCamera(ctx context.Context, id int64, c Camera) (Camera, error) {
	// Empty password in update means "keep existing".
	if c.Password == "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE cameras SET name=?, host=?, username=?, enabled=?
			WHERE id=?`,
			c.Name, c.Host, c.Username, boolToInt(c.Enabled), id)
		if err != nil {
			return Camera{}, err
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			UPDATE cameras SET name=?, host=?, username=?, password=?, enabled=?
			WHERE id=?`,
			c.Name, c.Host, c.Username, c.Password, boolToInt(c.Enabled), id)
		if err != nil {
			return Camera{}, err
		}
	}
	return s.GetCamera(ctx, id)
}

func (s *Store) DeleteCamera(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM cameras WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertEvent records a single camera event.
func (s *Store) InsertEvent(ctx context.Context, e Event) (Event, error) {
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO events (camera_id, code, action, channel_idx, data_json, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.CameraID, e.Code, e.Action, e.ChannelIdx, e.DataJSON, e.OccurredAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Event{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Event{}, err
	}
	e.ID = id
	return e, nil
}

// ListEventsFilter narrows ListEvents. Zero values are ignored.
type ListEventsFilter struct {
	CameraID int64
	Codes    []string
	Limit    int
	Before   time.Time // pagination cursor — return events strictly older than this
}

func (s *Store) ListEvents(ctx context.Context, f ListEventsFilter) ([]Event, error) {
	q := `SELECT e.id, e.camera_id, c.name, e.code, e.action, e.channel_idx, e.data_json, e.occurred_at
		FROM events e JOIN cameras c ON c.id = e.camera_id WHERE 1=1`
	var args []any
	if f.CameraID > 0 {
		q += " AND e.camera_id = ?"
		args = append(args, f.CameraID)
	}
	if len(f.Codes) > 0 {
		q += " AND e.code IN (" + placeholders(len(f.Codes)) + ")"
		for _, c := range f.Codes {
			args = append(args, c)
		}
	}
	if !f.Before.IsZero() {
		q += " AND e.occurred_at < ?"
		args = append(args, f.Before.UTC().Format(time.RFC3339Nano))
	}
	q += " ORDER BY e.occurred_at DESC, e.id DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var occurred string
		if err := rows.Scan(&e.ID, &e.CameraID, &e.CameraName, &e.Code, &e.Action, &e.ChannelIdx, &e.DataJSON, &occurred); err != nil {
			return nil, err
		}
		e.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		out = append(out, e)
	}
	return out, rows.Err()
}

// InsertRecording records a completed motion clip.
func (s *Store) InsertRecording(ctx context.Context, r Recording) (Recording, error) {
	var eventID any
	if r.EventID > 0 {
		eventID = r.EventID
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO recordings (camera_id, event_id, started_at, ended_at, duration_ms, path, size_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.CameraID, eventID,
		r.StartedAt.UTC().Format(time.RFC3339Nano),
		r.EndedAt.UTC().Format(time.RFC3339Nano),
		r.DurationMs, r.Path, r.SizeBytes)
	if err != nil {
		return Recording{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Recording{}, err
	}
	return s.GetRecording(ctx, id)
}

func (s *Store) GetRecording(ctx context.Context, id int64) (Recording, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT r.id, r.camera_id, c.name, COALESCE(r.event_id, 0),
		       r.started_at, r.ended_at, r.duration_ms, r.path, r.size_bytes, r.created_at
		FROM recordings r JOIN cameras c ON c.id = r.camera_id
		WHERE r.id = ?`, id)
	r, err := scanRecording(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Recording{}, ErrNotFound
	}
	return r, err
}

// ListRecordingsFilter narrows ListRecordings. Zero values are ignored.
type ListRecordingsFilter struct {
	CameraID int64
	EventID  int64
	Limit    int
	Before   time.Time
}

func (s *Store) ListRecordings(ctx context.Context, f ListRecordingsFilter) ([]Recording, error) {
	q := `SELECT r.id, r.camera_id, c.name, COALESCE(r.event_id, 0),
	             r.started_at, r.ended_at, r.duration_ms, r.path, r.size_bytes, r.created_at
		FROM recordings r JOIN cameras c ON c.id = r.camera_id WHERE 1=1`
	var args []any
	if f.CameraID > 0 {
		q += " AND r.camera_id = ?"
		args = append(args, f.CameraID)
	}
	if f.EventID > 0 {
		q += " AND r.event_id = ?"
		args = append(args, f.EventID)
	}
	if !f.Before.IsZero() {
		q += " AND r.started_at < ?"
		args = append(args, f.Before.UTC().Format(time.RFC3339Nano))
	}
	q += " ORDER BY r.started_at DESC, r.id DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recording
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanRecording(r rowScanner) (Recording, error) {
	var rec Recording
	var started, ended, created string
	if err := r.Scan(&rec.ID, &rec.CameraID, &rec.CameraName, &rec.EventID,
		&started, &ended, &rec.DurationMs, &rec.Path, &rec.SizeBytes, &created); err != nil {
		return Recording{}, err
	}
	rec.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	rec.EndedAt, _ = time.Parse(time.RFC3339Nano, ended)
	rec.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return rec, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCamera(r rowScanner) (Camera, error) {
	var c Camera
	var enabled int
	var created string
	if err := r.Scan(&c.ID, &c.Name, &c.Host, &c.Username, &c.Password, &enabled, &created); err != nil {
		return Camera{}, err
	}
	c.Enabled = enabled != 0
	c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return c, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, 2*n-1)
	for i := 0; i < n; i++ {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, '?')
	}
	return string(out)
}
