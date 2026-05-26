package domain

import (
	"context"

	"github.com/google/uuid"
)

// CredentialField declares one secret or config value a plugin needs in
// order to authenticate against its upstream API.
type CredentialField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Description string `json:"description,omitempty"`
}

// Credentials is the decrypted credential bundle for a single instance. The
// shape is defined by the plugin's CredentialSchema().
type Credentials map[string]string

// ActionDefinition describes one operator-invokable action exposed by a plugin
// against a configured instance. Destructive actions require approval / PBAC.
type ActionDefinition struct {
	Key             string            `json:"key"`
	Label           string            `json:"label"`
	Description     string            `json:"description"`
	Destructive     bool              `json:"destructive"`
	RequiresApproval bool             `json:"requires_approval"`
	Params          []ParamDefinition `json:"params"`
}

// Plugin is the root interface implemented by every integration. ID is
// stable and used as the plugin_id foreign key on integration_instances.
type Plugin interface {
	ID() string
	Name() string
	Description() string
	CredentialSchema() []CredentialField
	HealthCheck(ctx context.Context, creds Credentials) error
}

// ActionExecutor is implemented by plugins that expose imperative actions
// (suspend, wipe, reset_password, etc.) against an instance.
type ActionExecutor interface {
	Actions() []ActionDefinition
	Execute(ctx context.Context, creds Credentials, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any) (ActionResult, error)
}

// HoldProvider is implemented by plugins that can place or remove a legal
// hold for a custodian on the upstream system. Place and Remove must be
// idempotent: repeated calls for the same custodian must not error.
type HoldProvider interface {
	PlaceHold(ctx context.Context, creds Credentials, instanceID uuid.UUID, custodianEmail string) (ActionResult, error)
	RemoveHold(ctx context.Context, creds Credentials, instanceID uuid.UUID, custodianEmail string) (ActionResult, error)
}

// IdentityProvider is implemented by plugins that can resolve an email to
// an identity record on the upstream system. Returning a not-found error is
// expected behavior when the email is not present on that instance.
type IdentityProvider interface {
	LookupIdentity(ctx context.Context, creds Credentials, instanceID uuid.UUID, email string) (Identity, error)
}
