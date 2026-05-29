// Package scim implements the SCIM 2.0 (RFC 7644) handler logic for Warden's
// /scim/v2/* surface. Bearer token authentication is enforced by middleware
// (Agent 5) — the handlers assume the request is already authorized and
// focus on schema mapping between SCIM JSON and the users / scim_groups
// tables.
//
// The handlers exposed here are pure functions over (context, params): the
// HTTP routing layer wires them to chi / connect-go routes.
package scim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SCIM URN constants. The spec defines these strings; do not normalize or
// reformat them.
const (
	SchemaUser           = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaGroup          = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SchemaListResponse   = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaPatchOperation = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SchemaError          = "urn:ietf:params:scim:api:messages:2.0:Error"

	// ResourceTypeUser and ResourceTypeGroup populate the meta.resourceType
	// field of each emitted resource.
	ResourceTypeUser  = "User"
	ResourceTypeGroup = "Group"
)

// Meta is the SCIM "meta" sub-resource: timestamps, resource type, and a
// self-referential location URL. Location is left to the HTTP layer so the
// handlers do not need to know the base URL.
type Meta struct {
	ResourceType string    `json:"resourceType"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location,omitempty"`
	Version      string    `json:"version,omitempty"`
}

// Email is one entry of the SCIM "emails" complex multi-valued attribute.
type Email struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary,omitempty"`
	Type    string `json:"type,omitempty"`
}

// Name is the SCIM "name" complex attribute. Warden stores a single
// formatted name, so the GivenName / FamilyName fields are derived
// best-effort on output and ignored on input (Formatted wins).
type Name struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

// User is the SCIM User schema as Warden emits / accepts it. Unknown fields
// from the client are ignored; Warden does not implement extension schemas.
type User struct {
	Schemas    []string `json:"schemas"`
	ID         string   `json:"id,omitempty"`
	ExternalID string   `json:"externalId,omitempty"`
	UserName   string   `json:"userName"`
	Active     bool     `json:"active"`
	Name       Name     `json:"name,omitempty"`
	Emails     []Email  `json:"emails,omitempty"`
	Meta       Meta     `json:"meta,omitempty"`
}

// GroupMember is one entry of the SCIM "members" attribute on a Group.
type GroupMember struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

// Group is the SCIM Group schema as Warden emits / accepts it.
type Group struct {
	Schemas     []string      `json:"schemas"`
	ID          string        `json:"id,omitempty"`
	ExternalID  string        `json:"externalId,omitempty"`
	DisplayName string        `json:"displayName"`
	Members     []GroupMember `json:"members,omitempty"`
	Meta        Meta          `json:"meta,omitempty"`
}

// ListResponse is the envelope returned from list endpoints.
type ListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	ItemsPerPage int      `json:"itemsPerPage"`
	StartIndex   int      `json:"startIndex"`
	Resources    []any    `json:"Resources"`
}

// Error is the SCIM error response. Detail is the human-readable message;
// SCIMType is the optional machine-readable error category.
type Error struct {
	Schemas  []string `json:"schemas"`
	Detail   string   `json:"detail"`
	Status   string   `json:"status"`
	SCIMType string   `json:"scimType,omitempty"`
}

// NewError constructs an Error with the supplied HTTP status and detail.
func NewError(status int, detail string) Error {
	return Error{
		Schemas: []string{SchemaError},
		Detail:  detail,
		Status:  fmt.Sprintf("%d", status),
	}
}

// PatchOperation is one entry of a SCIM PATCH request body.
type PatchOperation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// PatchRequest is the body of an HTTP PATCH against a SCIM resource.
type PatchRequest struct {
	Schemas    []string         `json:"schemas"`
	Operations []PatchOperation `json:"Operations"`
}

// ErrNotFound is returned when no row matches the requested id / filter.
var ErrNotFound = errors.New("scim: resource not found")

// UserHandler implements the SCIM Users endpoints over the existing users
// table. SCIM "delete" is a soft delete: the row is kept and is_active is
// flipped to false. This preserves audit history and lets the IdP rehydrate
// the account by reactivating.
type UserHandler struct {
	Pool *pgxpool.Pool
}

// NewUserHandler constructs a UserHandler.
func NewUserHandler(pool *pgxpool.Pool) *UserHandler {
	return &UserHandler{Pool: pool}
}

// Create upserts the user by primary email (userName is treated as email if
// it contains an "@"). Returns the resulting SCIM resource.
func (h *UserHandler) Create(ctx context.Context, in User) (*User, error) {
	if h == nil || h.Pool == nil {
		return nil, errors.New("scim: user handler not initialized")
	}
	email := primaryEmail(in)
	if email == "" {
		return nil, errors.New("scim: user create: missing primary email")
	}
	name := strings.TrimSpace(in.Name.Formatted)
	if name == "" {
		name = strings.TrimSpace(in.UserName)
	}
	if name == "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			name = email[:at]
		} else {
			name = email
		}
	}

	var (
		id        uuid.UUID
		dbEmail   string
		dbName    string
		isActive  bool
		createdAt time.Time
		updatedAt time.Time
	)
	err := h.Pool.QueryRow(ctx, `
		INSERT INTO users (email, name, is_active, origin)
		VALUES ($1, $2, $3, 'scim')
		ON CONFLICT (email) DO UPDATE
		  SET name       = EXCLUDED.name,
		      is_active  = EXCLUDED.is_active,
		      origin     = 'scim',
		      updated_at = now()
		RETURNING id, email, name, is_active, created_at, updated_at
	`, email, name, in.Active).Scan(&id, &dbEmail, &dbName, &isActive, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scim: user create: %w", err)
	}
	return userResource(id, dbEmail, dbName, in.ExternalID, isActive, createdAt, updatedAt), nil
}

// Get returns the user identified by SCIM id (the users.id UUID).
func (h *UserHandler) Get(ctx context.Context, id string) (*User, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("scim: invalid id: %w", err)
	}
	var (
		email     string
		name      string
		isActive  bool
		createdAt time.Time
		updatedAt time.Time
	)
	err = h.Pool.QueryRow(ctx, `
		SELECT email, name, is_active, created_at, updated_at
		FROM users
		WHERE id = $1
	`, uid).Scan(&email, &name, &isActive, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scim: user get: %w", err)
	}
	return userResource(uid, email, name, "", isActive, createdAt, updatedAt), nil
}

// Update replaces the mutable fields (name, active) on an existing user.
// userName / email is treated as immutable here — SCIM operationally
// supports email changes, but identity providers typically issue a new
// resource when an email changes, and silently re-keying the row would
// detach audit history.
func (h *UserHandler) Update(ctx context.Context, id string, in User) (*User, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("scim: invalid id: %w", err)
	}
	name := strings.TrimSpace(in.Name.Formatted)
	if name == "" {
		name = strings.TrimSpace(in.UserName)
	}
	var (
		email     string
		dbName    string
		isActive  bool
		createdAt time.Time
		updatedAt time.Time
	)
	err = h.Pool.QueryRow(ctx, `
		UPDATE users
		SET name       = COALESCE(NULLIF($2, ''), name),
		    is_active  = $3,
		    updated_at = now()
		WHERE id = $1
		RETURNING email, name, is_active, created_at, updated_at
	`, uid, name, in.Active).Scan(&email, &dbName, &isActive, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scim: user update: %w", err)
	}
	return userResource(uid, email, dbName, in.ExternalID, isActive, createdAt, updatedAt), nil
}

// Delete is a soft delete: is_active = false. The row stays so that audit
// references remain resolvable.
func (h *UserHandler) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("scim: invalid id: %w", err)
	}
	tag, err := h.Pool.Exec(ctx, `
		UPDATE users
		SET is_active  = false,
		    updated_at = now()
		WHERE id = $1
	`, uid)
	if err != nil {
		return fmt.Errorf("scim: user delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns a paged list of users. startIndex is 1-based per SCIM. count
// is clamped to the [1, 200] range.
func (h *UserHandler) List(ctx context.Context, startIndex, count int) (*ListResponse, error) {
	if startIndex < 1 {
		startIndex = 1
	}
	if count <= 0 {
		count = 50
	}
	if count > 200 {
		count = 200
	}

	var total int
	if err := h.Pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM users`).Scan(&total); err != nil {
		return nil, fmt.Errorf("scim: user count: %w", err)
	}

	rows, err := h.Pool.Query(ctx, `
		SELECT id, email, name, is_active, created_at, updated_at
		FROM users
		ORDER BY email ASC
		LIMIT $1 OFFSET $2
	`, count, startIndex-1)
	if err != nil {
		return nil, fmt.Errorf("scim: user list: %w", err)
	}
	defer rows.Close()

	resources := []any{}
	for rows.Next() {
		var (
			id        uuid.UUID
			email     string
			name      string
			isActive  bool
			createdAt time.Time
			updatedAt time.Time
		)
		if err := rows.Scan(&id, &email, &name, &isActive, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scim: user scan: %w", err)
		}
		resources = append(resources, userResource(id, email, name, "", isActive, createdAt, updatedAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scim: user list rows: %w", err)
	}

	return &ListResponse{
		Schemas:      []string{SchemaListResponse},
		TotalResults: total,
		ItemsPerPage: len(resources),
		StartIndex:   startIndex,
		Resources:    resources,
	}, nil
}

// GroupHandler implements the SCIM Groups endpoints. Warden maps a SCIM
// group's displayName to scim_groups.name and stores its externalId. The
// optional role_id column links the group to an RBAC role: when a member is
// added to the group, the corresponding user_roles row is created so the
// member inherits the role's permissions.
type GroupHandler struct {
	Pool *pgxpool.Pool
}

// NewGroupHandler constructs a GroupHandler.
func NewGroupHandler(pool *pgxpool.Pool) *GroupHandler {
	return &GroupHandler{Pool: pool}
}

// Create inserts a new SCIM group row. Members supplied at create time are
// translated into user_roles rows when the group has a role_id; members
// without a corresponding users row are ignored (the IdP may sync users
// out-of-order).
func (h *GroupHandler) Create(ctx context.Context, in Group) (*Group, error) {
	if h == nil || h.Pool == nil {
		return nil, errors.New("scim: group handler not initialized")
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return nil, errors.New("scim: group create: missing displayName")
	}
	externalID := strings.TrimSpace(in.ExternalID)
	if externalID == "" {
		// SCIM allows externalId to be absent; we synthesize one from the
		// display name to satisfy the NOT NULL UNIQUE constraint on the
		// scim_groups.external_id column.
		externalID = "warden:" + displayName
	}

	var (
		id        uuid.UUID
		createdAt time.Time
		updatedAt time.Time
	)
	err := h.Pool.QueryRow(ctx, `
		INSERT INTO scim_groups (external_id, name)
		VALUES ($1, $2)
		ON CONFLICT (external_id) DO UPDATE
		  SET name       = EXCLUDED.name,
		      updated_at = now()
		RETURNING id, created_at, updated_at
	`, externalID, displayName).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scim: group create: %w", err)
	}

	if err := h.syncMembers(ctx, id, in.Members); err != nil {
		return nil, err
	}
	return groupResource(id, externalID, displayName, in.Members, createdAt, updatedAt), nil
}

// Get returns the SCIM group by id, including its current membership.
func (h *GroupHandler) Get(ctx context.Context, id string) (*Group, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("scim: invalid id: %w", err)
	}
	var (
		externalID string
		name       string
		createdAt  time.Time
		updatedAt  time.Time
	)
	err = h.Pool.QueryRow(ctx, `
		SELECT external_id, name, created_at, updated_at
		FROM scim_groups
		WHERE id = $1
	`, uid).Scan(&externalID, &name, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scim: group get: %w", err)
	}
	members, err := h.loadMembers(ctx, uid)
	if err != nil {
		return nil, err
	}
	return groupResource(uid, externalID, name, members, createdAt, updatedAt), nil
}

// Update replaces the displayName and member list. Members not in the
// supplied list have their role assignment revoked (only for the role bound
// to this group; other roles assigned to the user are untouched).
func (h *GroupHandler) Update(ctx context.Context, id string, in Group) (*Group, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("scim: invalid id: %w", err)
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return nil, errors.New("scim: group update: missing displayName")
	}
	var (
		externalID string
		createdAt  time.Time
		updatedAt  time.Time
	)
	err = h.Pool.QueryRow(ctx, `
		UPDATE scim_groups
		SET name       = $2,
		    updated_at = now()
		WHERE id = $1
		RETURNING external_id, created_at, updated_at
	`, uid, displayName).Scan(&externalID, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scim: group update: %w", err)
	}
	if err := h.syncMembers(ctx, uid, in.Members); err != nil {
		return nil, err
	}
	return groupResource(uid, externalID, displayName, in.Members, createdAt, updatedAt), nil
}

// Delete removes the group row and revokes the bound role from every member.
func (h *GroupHandler) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("scim: invalid id: %w", err)
	}
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("scim: group delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var roleID *uuid.UUID
	err = tx.QueryRow(ctx, `SELECT role_id FROM scim_groups WHERE id = $1`, uid).Scan(&roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scim: group delete lookup: %w", err)
	}
	if roleID != nil {
		if _, err := tx.Exec(ctx, `
			DELETE FROM user_roles WHERE role_id = $1
		`, *roleID); err != nil {
			return fmt.Errorf("scim: group delete role assignments: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM scim_groups WHERE id = $1`, uid); err != nil {
		return fmt.Errorf("scim: group delete: %w", err)
	}
	return tx.Commit(ctx)
}

// List returns a paged list of SCIM groups. Group members are *not*
// returned in list responses (per SCIM, this is implementation-defined and
// excluding them keeps responses small).
func (h *GroupHandler) List(ctx context.Context, startIndex, count int) (*ListResponse, error) {
	if startIndex < 1 {
		startIndex = 1
	}
	if count <= 0 {
		count = 50
	}
	if count > 200 {
		count = 200
	}
	var total int
	if err := h.Pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM scim_groups`).Scan(&total); err != nil {
		return nil, fmt.Errorf("scim: group count: %w", err)
	}

	rows, err := h.Pool.Query(ctx, `
		SELECT id, external_id, name, created_at, updated_at
		FROM scim_groups
		ORDER BY name ASC
		LIMIT $1 OFFSET $2
	`, count, startIndex-1)
	if err != nil {
		return nil, fmt.Errorf("scim: group list: %w", err)
	}
	defer rows.Close()

	resources := []any{}
	for rows.Next() {
		var (
			id         uuid.UUID
			externalID string
			name       string
			createdAt  time.Time
			updatedAt  time.Time
		)
		if err := rows.Scan(&id, &externalID, &name, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scim: group scan: %w", err)
		}
		resources = append(resources, groupResource(id, externalID, name, nil, createdAt, updatedAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scim: group rows: %w", err)
	}
	return &ListResponse{
		Schemas:      []string{SchemaListResponse},
		TotalResults: total,
		ItemsPerPage: len(resources),
		StartIndex:   startIndex,
		Resources:    resources,
	}, nil
}

// loadMembers reads the set of users currently assigned to the role bound
// to the supplied SCIM group. If the group has no role_id, returns empty.
func (h *GroupHandler) loadMembers(ctx context.Context, groupID uuid.UUID) ([]GroupMember, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT u.id::text, u.email
		FROM scim_groups g
		JOIN user_roles ur ON ur.role_id = g.role_id
		JOIN users u ON u.id = ur.user_id
		WHERE g.id = $1
		ORDER BY u.email ASC
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("scim: load members: %w", err)
	}
	defer rows.Close()
	out := []GroupMember{}
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.Value, &m.Display); err != nil {
			return nil, fmt.Errorf("scim: scan member: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scim: member rows: %w", err)
	}
	return out, nil
}

// syncMembers reconciles the group's membership with the supplied list. If
// the group has no role_id, members are ignored (the group has no RBAC
// effect until an admin links a role to it).
func (h *GroupHandler) syncMembers(ctx context.Context, groupID uuid.UUID, members []GroupMember) error {
	var roleID *uuid.UUID
	err := h.Pool.QueryRow(ctx, `SELECT role_id FROM scim_groups WHERE id = $1`, groupID).Scan(&roleID)
	if err != nil {
		return fmt.Errorf("scim: sync members: lookup role: %w", err)
	}
	if roleID == nil {
		return nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("scim: sync members tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE role_id = $1`, *roleID); err != nil {
		return fmt.Errorf("scim: sync members clear: %w", err)
	}
	for _, m := range members {
		uid, err := uuid.Parse(m.Value)
		if err != nil {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			VALUES ($1, $2)
			ON CONFLICT (user_id, role_id) DO NOTHING
		`, uid, *roleID); err != nil {
			return fmt.Errorf("scim: sync members insert: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// primaryEmail returns the lowercased primary email from a SCIM User. If no
// entry is flagged primary, the first email value is used; if there are no
// email entries, the userName is treated as an email when it contains "@".
func primaryEmail(u User) string {
	for _, e := range u.Emails {
		if e.Primary && strings.Contains(e.Value, "@") {
			return strings.ToLower(strings.TrimSpace(e.Value))
		}
	}
	for _, e := range u.Emails {
		if strings.Contains(e.Value, "@") {
			return strings.ToLower(strings.TrimSpace(e.Value))
		}
	}
	if strings.Contains(u.UserName, "@") {
		return strings.ToLower(strings.TrimSpace(u.UserName))
	}
	return ""
}

func userResource(id uuid.UUID, email, name, externalID string, active bool, createdAt, updatedAt time.Time) *User {
	parts := strings.SplitN(name, " ", 2)
	given := parts[0]
	family := ""
	if len(parts) == 2 {
		family = parts[1]
	}
	return &User{
		Schemas:    []string{SchemaUser},
		ID:         id.String(),
		ExternalID: externalID,
		UserName:   email,
		Active:     active,
		Name: Name{
			Formatted:  name,
			GivenName:  given,
			FamilyName: family,
		},
		Emails: []Email{
			{Value: email, Primary: true, Type: "work"},
		},
		Meta: Meta{
			ResourceType: ResourceTypeUser,
			Created:      createdAt,
			LastModified: updatedAt,
		},
	}
}

func groupResource(id uuid.UUID, externalID, displayName string, members []GroupMember, createdAt, updatedAt time.Time) *Group {
	return &Group{
		Schemas:     []string{SchemaGroup},
		ID:          id.String(),
		ExternalID:  externalID,
		DisplayName: displayName,
		Members:     members,
		Meta: Meta{
			ResourceType: ResourceTypeGroup,
			Created:      createdAt,
			LastModified: updatedAt,
		},
	}
}

// StatusForError maps a handler error to the appropriate HTTP status. The
// HTTP layer (Agent 5) uses this to format the SCIM error envelope.
func StatusForError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
