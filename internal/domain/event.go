package domain

import (
	"time"

	"github.com/google/uuid"
)

// AggregateType is the kind of aggregate an event applies to. The pair
// (AggregateType, AggregateID) uniquely identifies an event stream.
type AggregateType string

const (
	AggregateHold         AggregateType = "hold"
	AggregateHoldTemplate AggregateType = "hold_template"
	AggregateCustodian    AggregateType = "custodian"
	AggregateUser         AggregateType = "user"
	AggregateInstance     AggregateType = "instance"
	AggregateAction       AggregateType = "action"
	AggregateApproval     AggregateType = "approval"
	AggregateBreakGlass   AggregateType = "breakglass"
	AggregateIdentity     AggregateType = "identity"
)

// Event type string constants. Event names are stable strings; never rename
// without a migration.
const (
	// Hold lifecycle
	EventHoldCreated  = "hold.created"
	EventHoldUpdated  = "hold.updated"
	EventHoldReleased = "hold.released"
	EventHoldExpired  = "hold.expired"

	// Custodian membership
	EventCustodianAdded   = "custodian.added"
	EventCustodianRemoved = "custodian.removed"

	// Cascade state (per-provider hold placement)
	EventCascadeRequested = "cascade.requested"
	EventCascadeUpdated   = "cascade.updated"
	EventCascadeCompleted = "cascade.completed"
	EventCascadeFailed    = "cascade.failed"

	// Action execution against an integration
	EventActionExecuted = "action.executed"
	EventActionFailed   = "action.failed"

	// Break-glass override
	EventBreakGlassUsed     = "breakglass.used"
	EventBreakGlassReviewed = "breakglass.reviewed"

	// Approval workflow
	EventApprovalRequested = "approval.requested"
	EventApprovalDecided   = "approval.decided"
	EventApprovalExpired   = "approval.expired"

	// Hold template lifecycle
	EventHoldTemplateCreated = "hold_template.created"
	EventHoldTemplateUpdated = "hold_template.updated"
	EventHoldTemplateDeleted = "hold_template.deleted"
)

// Event is an immutable, append-only fact about an aggregate. Once written,
// an event is never updated or deleted.
type Event struct {
	ID            uuid.UUID      `json:"id"`
	AggregateType AggregateType  `json:"aggregate_type"`
	AggregateID   uuid.UUID      `json:"aggregate_id"`
	Version       int            `json:"version"`
	Type          string         `json:"type"`
	Payload       map[string]any `json:"payload"`
	ActorID       *uuid.UUID     `json:"actor_id,omitempty"`
	ActorType     ActorType      `json:"actor_type,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}
