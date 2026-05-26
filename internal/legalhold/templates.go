package legalhold

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

// CreateTemplateParams carries fields required to create a hold template.
type CreateTemplateParams struct {
	Name               string
	Description        string
	ProviderGlob       string
	BlockedActions     []string
	NotesTemplate      string
	ExpirationDays     *int
}

// CreateTemplate inserts a new hold template and emits a hold.updated event on
// the hold aggregate. (Templates don't have their own aggregate type, so we use
// the system user aggregate.)
func (s *Service) CreateTemplate(
	ctx context.Context,
	tx pgx.Tx,
	actor domain.Actor,
	params CreateTemplateParams,
) (*domain.HoldTemplate, error) {
	if strings.TrimSpace(params.Name) == "" {
		return nil, errors.New("legalhold: template name required")
	}
	if params.ProviderGlob == "" {
		params.ProviderGlob = "*"
	}

	tpl := &domain.HoldTemplate{}
	err := tx.QueryRow(ctx, `
		INSERT INTO hold_templates (name, description, provider_glob, blocked_actions, notes_template, expiration_days, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, description, provider_glob, blocked_actions, expiration_days, notes_template, created_by, created_at, updated_at
	`,
		params.Name,
		nullText(params.Description),
		params.ProviderGlob,
		params.BlockedActions,
		nullText(params.NotesTemplate),
		params.ExpirationDays,
		nullUUID(actor.ID),
	).Scan(
		&tpl.ID,
		&tpl.Name,
		&tpl.Description,
		&tpl.ProviderGlob,
		&tpl.BlockedActions,
		&tpl.ExpirationDays,
		&tpl.NotesTemplate,
		&tpl.CreatedBy,
		&tpl.CreatedAt,
		&tpl.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("legalhold: create template: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHold, tpl.ID, domain.EventHoldUpdated, actor, map[string]any{
		"action":      "template_created",
		"template_id": tpl.ID,
		"name":        tpl.Name,
	}); err != nil {
		return nil, err
	}

	return tpl, nil
}

// GetTemplate loads a single template by ID.
func (s *Service) GetTemplate(ctx context.Context, pool *pgxpool.Pool, templateID uuid.UUID) (*domain.HoldTemplate, error) {
	tpl := &domain.HoldTemplate{}
	err := pool.QueryRow(ctx, `
		SELECT id, name, description, provider_glob, blocked_actions, expiration_days, notes_template, created_by, created_at, updated_at
		FROM hold_templates WHERE id = $1
	`, templateID).Scan(
		&tpl.ID, &tpl.Name, &tpl.Description, &tpl.ProviderGlob,
		&tpl.BlockedActions, &tpl.ExpirationDays, &tpl.NotesTemplate,
		&tpl.CreatedBy, &tpl.CreatedAt, &tpl.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("legalhold: template %s not found", templateID)
		}
		return nil, fmt.Errorf("legalhold: get template: %w", err)
	}
	return tpl, nil
}

// ListTemplates returns all hold templates, sorted by name.
func (s *Service) ListTemplates(ctx context.Context, pool *pgxpool.Pool) ([]*domain.HoldTemplate, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, description, provider_glob, blocked_actions, expiration_days, notes_template, created_by, created_at, updated_at
		FROM hold_templates
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("legalhold: list templates: %w", err)
	}
	defer rows.Close()

	out := []*domain.HoldTemplate{}
	for rows.Next() {
		tpl := &domain.HoldTemplate{}
		if err := rows.Scan(
			&tpl.ID, &tpl.Name, &tpl.Description, &tpl.ProviderGlob,
			&tpl.BlockedActions, &tpl.ExpirationDays, &tpl.NotesTemplate,
			&tpl.CreatedBy, &tpl.CreatedAt, &tpl.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("legalhold: scan template: %w", err)
		}
		out = append(out, tpl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("legalhold: list templates rows: %w", err)
	}
	return out, nil
}

// UpdateTemplateParams carries the mutable fields of a hold template.
type UpdateTemplateParams struct {
	Name           string
	Description    string
	ProviderGlob   string
	BlockedActions []string
	NotesTemplate  string
	ExpirationDays *int
}

// UpdateTemplate replaces the mutable fields of an existing template.
func (s *Service) UpdateTemplate(
	ctx context.Context,
	tx pgx.Tx,
	actor domain.Actor,
	templateID uuid.UUID,
	params UpdateTemplateParams,
) (*domain.HoldTemplate, error) {
	if strings.TrimSpace(params.Name) == "" {
		return nil, errors.New("legalhold: template name required")
	}
	if params.ProviderGlob == "" {
		params.ProviderGlob = "*"
	}

	tpl := &domain.HoldTemplate{}
	err := tx.QueryRow(ctx, `
		UPDATE hold_templates
		SET name            = $2,
		    description     = $3,
		    provider_glob   = $4,
		    blocked_actions = $5,
		    notes_template  = $6,
		    expiration_days = $7,
		    updated_at      = now()
		WHERE id = $1
		RETURNING id, name, description, provider_glob, blocked_actions, expiration_days, notes_template, created_by, created_at, updated_at
	`,
		templateID,
		params.Name,
		nullText(params.Description),
		params.ProviderGlob,
		params.BlockedActions,
		nullText(params.NotesTemplate),
		params.ExpirationDays,
	).Scan(
		&tpl.ID, &tpl.Name, &tpl.Description, &tpl.ProviderGlob,
		&tpl.BlockedActions, &tpl.ExpirationDays, &tpl.NotesTemplate,
		&tpl.CreatedBy, &tpl.CreatedAt, &tpl.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("legalhold: template %s not found", templateID)
		}
		return nil, fmt.Errorf("legalhold: update template: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHold, templateID, domain.EventHoldUpdated, actor, map[string]any{
		"action":      "template_updated",
		"template_id": templateID,
		"name":        tpl.Name,
	}); err != nil {
		return nil, err
	}

	return tpl, nil
}

// DeleteTemplate removes a hold template. Holds that were created from the
// template retain their data; the foreign key is SET NULL on delete.
func (s *Service) DeleteTemplate(ctx context.Context, tx pgx.Tx, actor domain.Actor, templateID uuid.UUID) error {
	tag, err := tx.Exec(ctx, `DELETE FROM hold_templates WHERE id = $1`, templateID)
	if err != nil {
		return fmt.Errorf("legalhold: delete template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("legalhold: template %s not found", templateID)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHold, templateID, domain.EventHoldUpdated, actor, map[string]any{
		"action":      "template_deleted",
		"template_id": templateID,
	}); err != nil {
		return err
	}

	return nil
}

// DefaultExpiresAt computes the expiration timestamp from a template's
// ExpirationDays field. Returns nil when no expiration is configured.
func DefaultExpiresAt(tpl *domain.HoldTemplate, from time.Time) *time.Time {
	if tpl == nil || tpl.ExpirationDays == nil {
		return nil
	}
	t := from.Add(time.Duration(*tpl.ExpirationDays) * 24 * time.Hour)
	return &t
}
