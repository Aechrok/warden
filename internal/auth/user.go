package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/domain"
)

// ErrUserNotFound is returned by lookups that match no row.
var ErrUserNotFound = errors.New("auth: user not found")

// UserStore manages the operator account record. The OIDC callback flow
// calls GetOrCreate after a successful exchange; admin pages and the
// session-auth middleware call GetByID.
type UserStore struct{}

// NewUserStore constructs a UserStore.
func NewUserStore() *UserStore {
	return &UserStore{}
}

// GetOrCreate upserts the operator by email. If the row already exists and
// the name has drifted, the new name is written and updated_at is touched.
// is_active is left untouched on update — an admin may have deactivated the
// operator deliberately and a subsequent login should not silently undo that.
func (s *UserStore) GetOrCreate(ctx context.Context, tx pgx.Tx, email, name string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("auth: user upsert: empty email")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		// Fall back to the local-part of the email so the column is never empty.
		if at := strings.IndexByte(email, '@'); at > 0 {
			name = email[:at]
		} else {
			name = email
		}
	}

	var (
		id        uuid.UUID
		dbName    string
		isActive  bool
		createdAt time.Time
		updatedAt time.Time
	)
	// ON CONFLICT updates name only when it has actually changed, so we do
	// not bump updated_at on every login.
	err := tx.QueryRow(ctx, `
		INSERT INTO users (email, name, is_active)
		VALUES ($1, $2, true)
		ON CONFLICT (email) DO UPDATE
		  SET name       = CASE WHEN users.name <> EXCLUDED.name THEN EXCLUDED.name ELSE users.name END,
		      updated_at = CASE WHEN users.name <> EXCLUDED.name THEN now() ELSE users.updated_at END
		RETURNING id, name, is_active, created_at, updated_at
	`, email, name).Scan(&id, &dbName, &isActive, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("auth: user upsert: %w", err)
	}

	return &domain.User{
		ID:        id,
		Email:     email,
		Name:      dbName,
		IsActive:  isActive,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// GetByID looks up an operator account by primary key.
func (s *UserStore) GetByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*domain.User, error) {
	if pool == nil {
		return nil, errors.New("auth: get user: nil pool")
	}
	if id == uuid.Nil {
		return nil, errors.New("auth: get user: nil id")
	}
	u := &domain.User{}
	err := pool.QueryRow(ctx, `
		SELECT id, email, name, is_active, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Name, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: get user: %w", err)
	}
	return u, nil
}

// GetByEmail looks up an operator account by email (case-insensitive).
func (s *UserStore) GetByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*domain.User, error) {
	if pool == nil {
		return nil, errors.New("auth: get user: nil pool")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrUserNotFound
	}
	u := &domain.User{}
	err := pool.QueryRow(ctx, `
		SELECT id, email, name, is_active, created_at, updated_at
		FROM users
		WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.Name, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: get user by email: %w", err)
	}
	return u, nil
}

// SetActive flips the is_active flag and bumps updated_at. Used both by the
// admin UI and by SCIM delete (which is a soft delete).
func (s *UserStore) SetActive(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, active bool) error {
	if pool == nil {
		return errors.New("auth: set active: nil pool")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE users
		SET is_active  = $2,
		    updated_at = now()
		WHERE id = $1
	`, id, active)
	if err != nil {
		return fmt.Errorf("auth: set active: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateName changes the display name of an operator and bumps updated_at.
func (s *UserStore) UpdateName(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, name string) error {
	if pool == nil {
		return errors.New("auth: update name: nil pool")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("auth: update name: empty name")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE users
		SET name       = $2,
		    updated_at = now()
		WHERE id = $1
	`, id, name)
	if err != nil {
		return fmt.Errorf("auth: update name: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// Tenure returns the wall-clock time since the user's account was created.
// Used by the new_operator_probation PBAC policy.
func (s *UserStore) Tenure(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (time.Duration, error) {
	if pool == nil {
		return 0, errors.New("auth: tenure: nil pool")
	}
	var createdAt time.Time
	err := pool.QueryRow(ctx, `SELECT created_at FROM users WHERE id = $1`, id).Scan(&createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrUserNotFound
		}
		return 0, fmt.Errorf("auth: tenure: %w", err)
	}
	return time.Since(createdAt), nil
}

// ListActive returns a paginated list of active operator accounts ordered by
// email. limit must be > 0; offset may be 0 to start at the first row.
func (s *UserStore) ListActive(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]domain.User, error) {
	if pool == nil {
		return nil, errors.New("auth: list users: nil pool")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := pool.Query(ctx, `
		SELECT id, email, name, is_active, created_at, updated_at
		FROM users
		WHERE is_active = true
		ORDER BY email ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("auth: list users: %w", err)
	}
	defer rows.Close()

	out := []domain.User{}
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("auth: scan user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: list users rows: %w", err)
	}
	return out, nil
}
