package domain

import (
	"time"

	"github.com/google/uuid"
)

// Identity is a resolved person-on-a-provider record. Data carries the raw
// provider payload (Okta user, Google directory user, etc.) for display and
// downstream action use.
type Identity struct {
	Email        string         `json:"email"`
	DisplayName  string         `json:"display_name"`
	InstanceID   uuid.UUID      `json:"instance_id"`
	InstanceName string         `json:"instance_name"`
	Data         map[string]any `json:"data"`
	FetchedAt    time.Time      `json:"fetched_at"`
}

// IdentityResult pairs an Identity with the error encountered while
// resolving it, so a batch lookup across instances can report per-instance
// success or failure without aborting the entire request.
type IdentityResult struct {
	Identity Identity `json:"identity"`
	Err      error    `json:"-"`
}
