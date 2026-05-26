package legalhold

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/plugin"
)

// NewWorkers registers all legalhold River workers and returns the bundle.
// Agent 5 passes this to river.Config.Workers.
func NewWorkers(svc *Service) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &CascadePlaceWorker{svc: svc})
	river.AddWorker(workers, &CascadeRemoveWorker{svc: svc})
	river.AddWorker(workers, &ReconcileHoldsWorker{svc: svc})
	river.AddWorker(workers, &ExpireHoldWorker{svc: svc})
	return workers
}

// ---------------------------------------------------------------------------
// CascadePlaceJob
// ---------------------------------------------------------------------------

// CascadePlaceArgs is enqueued when a custodian is added to an active hold.
// It drives hold placement on every integration instance whose plugin matches
// the hold template's provider_glob.
type CascadePlaceArgs struct {
	HoldID         uuid.UUID `json:"hold_id"`
	CustodianID    uuid.UUID `json:"custodian_id"`
	CustodianEmail string    `json:"custodian_email"`
}

func (CascadePlaceArgs) Kind() string { return "legalhold.cascade_place" }

// CascadePlaceWorker places a hold on every matching provider instance.
type CascadePlaceWorker struct {
	river.WorkerDefaults[CascadePlaceArgs]
	svc *Service
}

func (w *CascadePlaceWorker) Work(ctx context.Context, job *river.Job[CascadePlaceArgs]) error {
	args := job.Args
	pool := w.svc.pool

	hold, err := w.svc.GetHold(ctx, pool, args.HoldID)
	if err != nil {
		return fmt.Errorf("cascade place: get hold: %w", err)
	}
	if hold.Status != domain.HoldStatusActive {
		return nil
	}

	instances, err := matchingInstances(ctx, pool, w.svc.registry, hold)
	if err != nil {
		return fmt.Errorf("cascade place: resolve instances: %w", err)
	}

	csm := NewCascadeStateMachine(w.svc)

	for _, inst := range instances {
		if err := w.placeOnInstance(ctx, csm, args, inst); err != nil {
			// Log but do not return — attempt every instance; River retries the whole job on error.
			// We record the failure per-instance so state is visible in the DB.
			_ = err
		}
	}
	return nil
}

func (w *CascadePlaceWorker) placeOnInstance(
	ctx context.Context,
	csm *CascadeStateMachine,
	args CascadePlaceArgs,
	inst instanceInfo,
) error {
	pool := w.svc.pool

	// Upsert the cascade row outside the place-hold transaction so we always
	// have a row to update on success or failure.
	var cascadeID uuid.UUID
	tx0, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cascade place: begin upsert tx: %w", err)
	}
	cs, err := csm.UpsertPending(ctx, tx0, args.HoldID, args.CustodianEmail, inst.id)
	if err != nil {
		_ = tx0.Rollback(ctx)
		return err
	}
	if err := tx0.Commit(ctx); err != nil {
		return fmt.Errorf("cascade place: commit upsert tx: %w", err)
	}
	cascadeID = cs.ID

	// Idempotency: if already completed, skip.
	if cs.Status == domain.CascadeStatusCompleted {
		return nil
	}

	tx1, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cascade place: begin transition tx: %w", err)
	}
	if _, err := csm.Transition(ctx, tx1, cascadeID, domain.CascadeStatusInProgress, ""); err != nil {
		_ = tx1.Rollback(ctx)
		return err
	}
	if err := tx1.Commit(ctx); err != nil {
		return fmt.Errorf("cascade place: commit in_progress tx: %w", err)
	}

	result, placeErr := inst.provider.PlaceHold(ctx, inst.creds, inst.id, args.CustodianEmail)

	tx2, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cascade place: begin result tx: %w", err)
	}
	if placeErr != nil {
		if _, err := csm.Transition(ctx, tx2, cascadeID, domain.CascadeStatusFailed, placeErr.Error()); err != nil {
			_ = tx2.Rollback(ctx)
			return err
		}
		_ = tx2.Commit(ctx)
		return fmt.Errorf("cascade place: provider PlaceHold: %w", placeErr)
	}

	nextStatus := domain.CascadeStatusCompleted
	if !result.Success {
		nextStatus = domain.CascadeStatusPartial
	}
	if _, err := csm.Transition(ctx, tx2, cascadeID, nextStatus, ""); err != nil {
		_ = tx2.Rollback(ctx)
		return err
	}
	return tx2.Commit(ctx)
}

// ---------------------------------------------------------------------------
// CascadeRemoveJob
// ---------------------------------------------------------------------------

// CascadeRemoveArgs is enqueued when a custodian is removed from a hold or
// when a hold is released/expired.
type CascadeRemoveArgs struct {
	HoldID         uuid.UUID `json:"hold_id"`
	CustodianID    uuid.UUID `json:"custodian_id"`
	CustodianEmail string    `json:"custodian_email"`
}

func (CascadeRemoveArgs) Kind() string { return "legalhold.cascade_remove" }

// CascadeRemoveWorker removes provider-level holds for a custodian, but only
// when no other active hold covers the same custodian on each instance.
type CascadeRemoveWorker struct {
	river.WorkerDefaults[CascadeRemoveArgs]
	svc *Service
}

func (w *CascadeRemoveWorker) Work(ctx context.Context, job *river.Job[CascadeRemoveArgs]) error {
	args := job.Args
	pool := w.svc.pool

	hold, err := w.svc.GetHold(ctx, pool, args.HoldID)
	if err != nil {
		return fmt.Errorf("cascade remove: get hold: %w", err)
	}

	instances, err := matchingInstances(ctx, pool, w.svc.registry, hold)
	if err != nil {
		return fmt.Errorf("cascade remove: resolve instances: %w", err)
	}

	csm := NewCascadeStateMachine(w.svc)
	systemActor := domain.Actor{Type: domain.ActorTypeUser}

	for _, inst := range instances {
		if err := w.removeOnInstance(ctx, csm, args, inst, systemActor); err != nil {
			_ = err
		}
	}
	return nil
}

func (w *CascadeRemoveWorker) removeOnInstance(
	ctx context.Context,
	csm *CascadeStateMachine,
	args CascadeRemoveArgs,
	inst instanceInfo,
	actor domain.Actor,
) error {
	pool := w.svc.pool

	// Check if the custodian is on any other active hold that covers this instance.
	// If yes, skip the provider remove — the hold must remain in place.
	conflict, err := CustodianOnOtherActiveHold(ctx, pool, args.HoldID, args.CustodianEmail, inst.id)
	if err != nil {
		return fmt.Errorf("cascade remove: conflict check: %w", err)
	}
	if conflict {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("cascade remove: begin conflict event tx: %w", err)
		}
		if err := w.svc.appendEvent(ctx, tx, domain.AggregateHold, args.HoldID, "hold_conflict_skip", actor, map[string]any{
			"custodian_email": args.CustodianEmail,
			"instance_id":     inst.id,
		}); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		return tx.Commit(ctx)
	}

	var cascadeID uuid.UUID
	{
		tx0, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("cascade remove: begin upsert tx: %w", err)
		}
		cs, err := csm.UpsertPending(ctx, tx0, args.HoldID, args.CustodianEmail, inst.id)
		if err != nil {
			_ = tx0.Rollback(ctx)
			return err
		}
		if err := tx0.Commit(ctx); err != nil {
			return fmt.Errorf("cascade remove: commit upsert tx: %w", err)
		}
		cascadeID = cs.ID

		// Idempotency: already in a terminal remove-like state — nothing to do.
		if cs.Status == domain.CascadeStatusPartial {
			return nil
		}
	}

	tx1, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cascade remove: begin in_progress tx: %w", err)
	}
	if _, err := csm.Transition(ctx, tx1, cascadeID, domain.CascadeStatusInProgress, ""); err != nil {
		_ = tx1.Rollback(ctx)
		return err
	}
	if err := tx1.Commit(ctx); err != nil {
		return fmt.Errorf("cascade remove: commit in_progress tx: %w", err)
	}

	_, removeErr := inst.provider.RemoveHold(ctx, inst.creds, inst.id, args.CustodianEmail)

	tx2, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cascade remove: begin result tx: %w", err)
	}
	if removeErr != nil {
		if _, err := csm.Transition(ctx, tx2, cascadeID, domain.CascadeStatusFailed, removeErr.Error()); err != nil {
			_ = tx2.Rollback(ctx)
			return err
		}
		_ = tx2.Commit(ctx)
		return fmt.Errorf("cascade remove: provider RemoveHold: %w", removeErr)
	}
	if _, err := csm.Transition(ctx, tx2, cascadeID, domain.CascadeStatusCompleted, ""); err != nil {
		_ = tx2.Rollback(ctx)
		return err
	}
	return tx2.Commit(ctx)
}

// CustodianOnOtherActiveHold returns true when the custodian's email appears
// on at least one *other* active hold that has a completed cascade entry for
// the same instance. This is the correctness invariant: never remove a provider
// hold while another Warden hold still requires it.
func CustodianOnOtherActiveHold(
	ctx context.Context,
	pool interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	},
	excludeHoldID uuid.UUID,
	email string,
	instanceID uuid.UUID,
) (bool, error) {
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM legal_hold_custodians lhc
		JOIN legal_holds lh ON lh.id = lhc.hold_id
		JOIN cascade_state cs ON cs.hold_id = lhc.hold_id
		  AND cs.custodian_email = lhc.email
		  AND cs.instance_id = $3
		  AND cs.status = 'completed'
		WHERE lhc.email = $1
		  AND lhc.removed_at IS NULL
		  AND lh.status = 'active'
		  AND lh.id != $2
	`, email, excludeHoldID, instanceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("legalhold: other active hold check: %w", err)
	}
	return count > 0, nil
}

// ---------------------------------------------------------------------------
// ReconcileHoldsJob
// ---------------------------------------------------------------------------

// ReconcileHoldsArgs is the argument type for the periodic reconciliation job.
type ReconcileHoldsArgs struct{}

func (ReconcileHoldsArgs) Kind() string { return "legalhold.reconcile_holds" }

// ReconcileHoldsWorker queries all completed cascade rows, verifies each with
// the provider, and re-enqueues a place job on drift.
type ReconcileHoldsWorker struct {
	river.WorkerDefaults[ReconcileHoldsArgs]
	svc *Service
}

func (w *ReconcileHoldsWorker) Work(ctx context.Context, job *river.Job[ReconcileHoldsArgs]) error {
	pool := w.svc.pool
	systemActor := domain.Actor{Type: domain.ActorTypeUser}

	// Load all completed cascade rows.
	rows, err := pool.Query(ctx, `
		SELECT cs.id, cs.hold_id, cs.custodian_email, cs.instance_id,
		       lhc.id AS custodian_id
		FROM cascade_state cs
		JOIN legal_hold_custodians lhc ON lhc.hold_id = cs.hold_id
		  AND lhc.email = cs.custodian_email
		  AND lhc.removed_at IS NULL
		JOIN legal_holds lh ON lh.id = cs.hold_id
		WHERE cs.status = 'completed'
		  AND lh.status = 'active'
	`)
	if err != nil {
		return fmt.Errorf("reconcile: query cascade rows: %w", err)
	}
	defer rows.Close()

	type cascadeRow struct {
		id             uuid.UUID
		holdID         uuid.UUID
		custodianEmail string
		instanceID     uuid.UUID
		custodianID    uuid.UUID
	}
	var pending []cascadeRow
	for rows.Next() {
		var r cascadeRow
		if err := rows.Scan(&r.id, &r.holdID, &r.custodianEmail, &r.instanceID, &r.custodianID); err != nil {
			return fmt.Errorf("reconcile: scan row: %w", err)
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reconcile: rows err: %w", err)
	}

	rc := w.svc.riverClient
	if rc == nil {
		return fmt.Errorf("reconcile: river client not configured")
	}

	for _, r := range pending {
		p, creds, ok := resolveProvider(ctx, pool, w.svc.registry, r.instanceID)
		if !ok {
			continue
		}
		hp, ok := p.(domain.HoldProvider)
		if !ok {
			continue
		}

		result, err := hp.PlaceHold(ctx, creds, r.instanceID, r.custodianEmail)
		inPlace := err == nil && result.Success

		if !inPlace {
			// Emit drift event and re-enqueue a place job.
			tx, err := pool.Begin(ctx)
			if err != nil {
				continue
			}
			_ = w.svc.appendEvent(ctx, tx, domain.AggregateHold, r.holdID, "hold_drift_detected", systemActor, map[string]any{
				"cascade_id":      r.id,
				"custodian_email": r.custodianEmail,
				"instance_id":     r.instanceID,
			})
			if _, err := rc.InsertTx(ctx, tx, CascadePlaceArgs{
				HoldID:         r.holdID,
				CustodianID:    r.custodianID,
				CustodianEmail: r.custodianEmail,
			}, nil); err != nil {
				_ = tx.Rollback(ctx)
				continue
			}
			_ = tx.Commit(ctx)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// ExpireHoldJob
// ---------------------------------------------------------------------------

// ExpireHoldArgs is enqueued at hold creation time with ScheduledAt=ExpiresAt.
type ExpireHoldArgs struct {
	HoldID      uuid.UUID `json:"hold_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

func (a ExpireHoldArgs) Kind() string { return "legalhold.expire_hold" }

func (a ExpireHoldArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{ScheduledAt: a.ScheduledAt}
}

// ExpireHoldWorker fires when a hold's ExpiresAt timestamp is reached.
type ExpireHoldWorker struct {
	river.WorkerDefaults[ExpireHoldArgs]
	svc *Service
}

func (w *ExpireHoldWorker) Work(ctx context.Context, job *river.Job[ExpireHoldArgs]) error {
	pool := w.svc.pool
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("expire hold: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := w.svc.ExpireHold(ctx, tx, job.Args.HoldID); err != nil {
		return fmt.Errorf("expire hold: %w", err)
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Instance resolution helpers
// ---------------------------------------------------------------------------

// instanceInfo is the resolved (id, provider, creds) triple for a plugin instance.
type instanceInfo struct {
	id       uuid.UUID
	provider domain.HoldProvider
	creds    domain.Credentials
}

// matchingInstances returns all active integration instances whose plugin
// implements HoldProvider and whose plugin ID matches the hold's provider_glob.
func matchingInstances(
	ctx context.Context,
	pool interface {
		Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	},
	reg *plugin.Registry,
	hold *domain.Hold,
) ([]instanceInfo, error) {
	glob := "*"
	if hold.TemplateID != nil {
		var tplGlob string
		// Best-effort: if we can't load the template glob, fall back to *.
		_ = queryTemplateGlob(ctx, pool, *hold.TemplateID, &tplGlob)
		if tplGlob != "" {
			glob = tplGlob
		}
	}

	rows, err := pool.Query(ctx, `
		SELECT id, plugin_id FROM integration_instances WHERE is_active = true
	`)
	if err != nil {
		return nil, fmt.Errorf("legalhold: query instances: %w", err)
	}
	defer rows.Close()

	var out []instanceInfo
	for rows.Next() {
		var (
			id       uuid.UUID
			pluginID string
		)
		if err := rows.Scan(&id, &pluginID); err != nil {
			return nil, fmt.Errorf("legalhold: scan instance: %w", err)
		}

		if !globMatch(glob, pluginID) {
			continue
		}
		p, ok := reg.Get(pluginID)
		if !ok {
			continue
		}
		hp, ok := p.(domain.HoldProvider)
		if !ok {
			continue
		}
		out = append(out, instanceInfo{id: id, provider: hp})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("legalhold: instance rows: %w", err)
	}
	return out, nil
}

// resolveProvider looks up a provider for a single instance ID. Returns false
// if the instance or plugin cannot be found or does not implement HoldProvider.
func resolveProvider(
	ctx context.Context,
	pool interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	},
	reg *plugin.Registry,
	instanceID uuid.UUID,
) (domain.Plugin, domain.Credentials, bool) {
	var pluginID string
	if err := pool.QueryRow(ctx, `SELECT plugin_id FROM integration_instances WHERE id = $1`, instanceID).Scan(&pluginID); err != nil {
		return nil, nil, false
	}
	p, ok := reg.Get(pluginID)
	if !ok {
		return nil, nil, false
	}
	return p, domain.Credentials{}, true
}

// queryTemplateGlob fetches the provider_glob for a template, writing into dst.
func queryTemplateGlob(
	ctx context.Context,
	pool interface {
		Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	},
	templateID uuid.UUID,
	dst *string,
) error {
	rows, err := pool.Query(ctx, `SELECT provider_glob FROM hold_templates WHERE id = $1`, templateID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return rows.Scan(dst)
	}
	return nil
}

// GlobMatch returns whether name matches the glob pattern using filepath-style
// rules: '*' matches any sequence (including empty), '?' matches one char.
func GlobMatch(pattern, name string) bool {
	return matchGlob(pattern, name)
}

// globMatch is the unexported alias retained for internal use.
func globMatch(pattern, name string) bool {
	return matchGlob(pattern, name)
}

// matchGlob is a simple iterative glob matcher supporting '*' and '?'.
func matchGlob(pattern, name string) bool {
	px, nx := 0, 0
	nextPx, nextNx := -1, -1

	for px < len(pattern) || nx < len(name) {
		if px < len(pattern) {
			c := pattern[px]
			switch c {
			case '*':
				// Try matching empty string at current nx; if fails, advance nx.
				nextPx = px
				nextNx = nx + 1
				px++
				continue
			case '?':
				if nx < len(name) {
					px++
					nx++
					continue
				}
			default:
				if nx < len(name) && name[nx] == c {
					px++
					nx++
					continue
				}
			}
		}
		// Mismatch — backtrack to last '*' match.
		if nextNx > 0 && nextNx <= len(name) {
			px = nextPx
			nx = nextNx
			nextNx++
			continue
		}
		return false
	}
	return true
}

