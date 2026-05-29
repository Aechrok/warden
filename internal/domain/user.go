package domain

import (
	"time"

	"github.com/google/uuid"
)

// User is an operator account in Warden. Identified by a stable UUID; email
// is the natural key used for upsert during OIDC and SCIM provisioning.
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	Origin    string    `json:"origin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
