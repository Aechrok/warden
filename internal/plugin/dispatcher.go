package plugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/domain"
)

// ErrInstanceInactive is returned when an action is attempted against an
// instance whose is_active flag is false.
var ErrInstanceInactive = errors.New("plugin dispatcher: instance is inactive")

// ErrActionNotSupported is returned when a plugin does not implement
// ActionExecutor (or does not declare the requested action key).
var ErrActionNotSupported = errors.New("plugin dispatcher: action not supported by plugin")

// ErrHoldNotSupported is returned when a hold operation targets a plugin
// that does not implement HoldProvider.
var ErrHoldNotSupported = errors.New("plugin dispatcher: hold not supported by plugin")

// ErrHoldNotConfigured is returned by hold-capable plugins that require a
// linked partner instance (e.g. google_workspace -> google_vault) when the
// link is not configured.
var ErrHoldNotConfigured = errors.New("plugin dispatcher: hold not configured for instance")

// ErrIdentityNotSupported is returned when an identity lookup targets a
// plugin that does not implement IdentityProvider.
var ErrIdentityNotSupported = errors.New("plugin dispatcher: identity lookup not supported by plugin")

// Dispatcher routes execution, identity, hold, and health-check requests to
// the appropriate registered plugin for a given integration instance.
type Dispatcher struct {
	registry *Registry
	resolver *CredentialResolver
	pool     *pgxpool.Pool
}

// NewDispatcher wires a Dispatcher to a registry, credential resolver, and
// database pool. The pool is currently only used indirectly (via the
// resolver), but is kept on the struct so future health-status writes can
// hit the integration_instances table without re-wiring callers.
func NewDispatcher(reg *Registry, res *CredentialResolver, pool *pgxpool.Pool) *Dispatcher {
	if reg == nil {
		reg = global
	}
	return &Dispatcher{registry: reg, resolver: res, pool: pool}
}

// Execute runs an action on the plugin backing instanceID.
func (d *Dispatcher) Execute(ctx context.Context, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any) (domain.ActionResult, error) {
	row, creds, p, err := d.prepare(ctx, instanceID, true)
	if err != nil {
		return domain.ActionResult{}, err
	}
	exec, ok := p.(domain.ActionExecutor)
	if !ok {
		return domain.ActionResult{}, fmt.Errorf("%w: plugin %q", ErrActionNotSupported, p.ID())
	}
	if !hasAction(exec, actionKey) {
		return domain.ActionResult{}, fmt.Errorf("%w: plugin %q has no action %q", ErrActionNotSupported, p.ID(), actionKey)
	}
	return exec.Execute(ctx, creds, row.ID, actionKey, targetEmail, params)
}

// GetIdentity resolves an email to the upstream identity record on the
// instance's plugin.
func (d *Dispatcher) GetIdentity(ctx context.Context, instanceID uuid.UUID, email string) (domain.Identity, error) {
	row, creds, p, err := d.prepare(ctx, instanceID, true)
	if err != nil {
		return domain.Identity{}, err
	}
	ip, ok := p.(domain.IdentityProvider)
	if !ok {
		return domain.Identity{}, fmt.Errorf("%w: plugin %q", ErrIdentityNotSupported, p.ID())
	}
	id, err := ip.LookupIdentity(ctx, creds, row.ID, email)
	if err != nil {
		return domain.Identity{}, err
	}
	if id.InstanceID == uuid.Nil {
		id.InstanceID = row.ID
	}
	if id.InstanceName == "" {
		id.InstanceName = row.Name
	}
	return id, nil
}

// PlaceHold delegates to the plugin's HoldProvider.PlaceHold.
func (d *Dispatcher) PlaceHold(ctx context.Context, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	row, creds, p, err := d.prepare(ctx, instanceID, true)
	if err != nil {
		return domain.ActionResult{}, err
	}
	hp, ok := p.(domain.HoldProvider)
	if !ok {
		return domain.ActionResult{}, fmt.Errorf("%w: plugin %q", ErrHoldNotSupported, p.ID())
	}
	return hp.PlaceHold(ctx, creds, row.ID, custodianEmail)
}

// RemoveHold delegates to the plugin's HoldProvider.RemoveHold.
func (d *Dispatcher) RemoveHold(ctx context.Context, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	row, creds, p, err := d.prepare(ctx, instanceID, true)
	if err != nil {
		return domain.ActionResult{}, err
	}
	hp, ok := p.(domain.HoldProvider)
	if !ok {
		return domain.ActionResult{}, fmt.Errorf("%w: plugin %q", ErrHoldNotSupported, p.ID())
	}
	return hp.RemoveHold(ctx, creds, row.ID, custodianEmail)
}

// HealthCheck runs the plugin's HealthCheck against the instance credentials.
// Honors is_active=false by returning ErrInstanceInactive so callers can
// distinguish "operator disabled" from "upstream broken".
func (d *Dispatcher) HealthCheck(ctx context.Context, instanceID uuid.UUID) error {
	row, creds, p, err := d.prepare(ctx, instanceID, false)
	if err != nil {
		return err
	}
	if !row.IsActive {
		return ErrInstanceInactive
	}
	return p.HealthCheck(ctx, creds)
}

// prepare loads the instance row, resolves credentials, and looks up the
// plugin. If enforceActive is true and the instance is inactive, returns
// ErrInstanceInactive without contacting the plugin.
func (d *Dispatcher) prepare(ctx context.Context, instanceID uuid.UUID, enforceActive bool) (InstanceRow, domain.Credentials, domain.Plugin, error) {
	row, err := d.resolver.LookupInstance(ctx, instanceID)
	if err != nil {
		return InstanceRow{}, nil, nil, err
	}
	if enforceActive && !row.IsActive {
		return InstanceRow{}, nil, nil, ErrInstanceInactive
	}
	p, ok := d.registry.Get(row.PluginID)
	if !ok {
		return InstanceRow{}, nil, nil, fmt.Errorf("%w: %s", ErrPluginNotFound, row.PluginID)
	}
	creds, err := d.resolver.ResolveRow(row)
	if err != nil {
		return InstanceRow{}, nil, nil, err
	}
	return row, creds, p, nil
}

func hasAction(exec domain.ActionExecutor, key string) bool {
	for _, a := range exec.Actions() {
		if a.Key == key {
			return true
		}
	}
	return false
}
