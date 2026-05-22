package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AdminCredentials is the (currently single-user) admin row used to log
// into the web UI.
type AdminCredentials struct {
	ID                  int64
	Username            string
	MustChangePassword  bool
	UpdatedAt           time.Time
}

// EnsureAdminSeeded inserts a default admin/admin row with the
// must-change-password flag set if the row doesn't already exist. Called
// after migrate() during Open().
func (s *Store) EnsureAdminSeeded(ctx context.Context) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_credentials`).Scan(&n); err != nil {
		return fmt.Errorf("count admin: %w", err)
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO admin_credentials (id, username, password_hash, must_change_password)
		VALUES (1, 'admin', ?, 1)`, string(hash))
	return err
}

// GetAdmin returns the admin row without the hash. The hash is only used
// internally; never exposed.
func (s *Store) GetAdmin(ctx context.Context) (AdminCredentials, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, must_change_password, updated_at
		FROM admin_credentials WHERE id = 1`)
	var a AdminCredentials
	var must int
	var updated string
	if err := row.Scan(&a.ID, &a.Username, &must, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AdminCredentials{}, ErrNotFound
		}
		return AdminCredentials{}, err
	}
	a.MustChangePassword = must != 0
	a.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
	return a, nil
}

// VerifyAdminPassword returns the admin row on success or an error if the
// password doesn't match. Constant-time inside bcrypt.
func (s *Store) VerifyAdminPassword(ctx context.Context, username, password string) (AdminCredentials, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, must_change_password, updated_at
		FROM admin_credentials WHERE id = 1`)
	var (
		a       AdminCredentials
		hash    string
		must    int
		updated string
	)
	if err := row.Scan(&a.ID, &a.Username, &hash, &must, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AdminCredentials{}, ErrNotFound
		}
		return AdminCredentials{}, err
	}
	// Username mismatch: still run bcrypt to keep response time uniform.
	usernameOK := a.Username == username
	pwErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if !usernameOK || pwErr != nil {
		return AdminCredentials{}, ErrBadCredentials
	}
	a.MustChangePassword = must != 0
	a.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
	return a, nil
}

// UpdateAdminPassword rehashes and stores a new password, clearing the
// must-change-password flag.
func (s *Store) UpdateAdminPassword(ctx context.Context, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE admin_credentials
		SET password_hash = ?, must_change_password = 0, updated_at = datetime('now')
		WHERE id = 1`, string(hash))
	return err
}

// ErrBadCredentials is returned for any login failure (wrong username,
// wrong password, or missing row). Callers should not distinguish.
var ErrBadCredentials = errors.New("invalid credentials")
