package domain

import "github.com/google/uuid"

// ActorType describes who performed an action recorded in the event store.
type ActorType string

const (
	// ActorTypeUser identifies a human operator authenticated via session.
	ActorTypeUser ActorType = "user"
	// ActorTypeToken identifies an API token principal.
	ActorTypeToken ActorType = "token"
)

// Actor is the principal attributed to an event or action. ID and Email may
// be zero values when the actor is a system process; in that case Type should
// still be set to the closest semantic value.
type Actor struct {
	ID    uuid.UUID `json:"id"`
	Type  ActorType `json:"type"`
	Email string    `json:"email"`
}
