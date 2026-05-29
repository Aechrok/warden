package domain

import (
	"time"

	"github.com/google/uuid"
)

// HoldStatus is the lifecycle state of a legal hold.
type HoldStatus string

const (
	HoldStatusActive   HoldStatus = "active"
	HoldStatusReleased HoldStatus = "released"
	HoldStatusExpired  HoldStatus = "expired"
)

// CascadeStatus is the per-custodian, per-provider state of a hold cascade.
type CascadeStatus string

const (
	CascadeStatusPending    CascadeStatus = "pending"
	CascadeStatusInProgress CascadeStatus = "in_progress"
	CascadeStatusCompleted  CascadeStatus = "completed"
	CascadeStatusPartial    CascadeStatus = "partial"
	CascadeStatusFailed     CascadeStatus = "failed"
)

// HoldTemplate is a reusable blueprint for creating holds with consistent
// scope, expiration, and blocked-action policy.
type HoldTemplate struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	ProviderGlob   string     `json:"provider_glob"`
	BlockedActions []string   `json:"blocked_actions"`
	ExpirationDays *int       `json:"expiration_days,omitempty"`
	NotesTemplate  string     `json:"notes_template"`
	IsDefault      bool       `json:"is_default"`
	CreatedBy      *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Hold is a legal hold applied to one or more custodians, cascaded to
// downstream provider instances via the cascade state machine.
type Hold struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	TemplateID  *uuid.UUID `json:"template_id,omitempty"`
	Status      HoldStatus `json:"status"`
	PlacedBy    *uuid.UUID `json:"placed_by,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Custodian is an identity (by email) currently or formerly subject to a hold.
// Removal is soft (RemovedAt set) so that the audit trail is preserved.
type Custodian struct {
	ID         uuid.UUID  `json:"id"`
	HoldID     uuid.UUID  `json:"hold_id"`
	Email      string     `json:"email"`
	AddedBy    *uuid.UUID `json:"added_by,omitempty"`
	RemovedAt  *time.Time `json:"removed_at,omitempty"`
	RemovedBy  *uuid.UUID `json:"removed_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CascadeState tracks a single (hold, custodian, instance) tuple as it moves
// from pending through completed or failed.
type CascadeState struct {
	ID             uuid.UUID     `json:"id"`
	HoldID         uuid.UUID     `json:"hold_id"`
	CustodianEmail string        `json:"custodian_email"`
	InstanceID     uuid.UUID     `json:"instance_id"`
	Status         CascadeStatus `json:"status"`
	LastError      string        `json:"last_error,omitempty"`
	Attempts       int           `json:"attempts"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}
