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
	Name           string
	Description    string
	ProviderGlob   string
	BlockedActions []string
	NotesTemplate  string
	ExpirationDays *int
	IsDefault      bool
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

	if params.BlockedActions == nil {
		params.BlockedActions = []string{}
	}
	if params.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE hold_templates SET is_default = false WHERE is_default = true`); err != nil {
			return nil, fmt.Errorf("legalhold: clear default template: %w", err)
		}
	}

	tpl := &domain.HoldTemplate{}
	err := tx.QueryRow(ctx, `
		INSERT INTO hold_templates (name, description, provider_glob, blocked_actions, notes_template, expiration_days, is_default, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, COALESCE(description, ''), provider_glob, blocked_actions, expiration_days, COALESCE(notes_template, ''), is_default, created_by, created_at, updated_at
	`,
		params.Name,
		nullText(params.Description),
		params.ProviderGlob,
		params.BlockedActions,
		nullText(params.NotesTemplate),
		params.ExpirationDays,
		params.IsDefault,
		nullUUID(actor.ID),
	).Scan(
		&tpl.ID,
		&tpl.Name,
		&tpl.Description,
		&tpl.ProviderGlob,
		&tpl.BlockedActions,
		&tpl.ExpirationDays,
		&tpl.NotesTemplate,
		&tpl.IsDefault,
		&tpl.CreatedBy,
		&tpl.CreatedAt,
		&tpl.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("legalhold: create template: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHoldTemplate, tpl.ID, domain.EventHoldTemplateCreated, actor, map[string]any{
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
		SELECT id, name, COALESCE(description, ''), provider_glob, blocked_actions, expiration_days, COALESCE(notes_template, ''), is_default, created_by, created_at, updated_at
		FROM hold_templates WHERE id = $1
	`, templateID).Scan(
		&tpl.ID, &tpl.Name, &tpl.Description, &tpl.ProviderGlob,
		&tpl.BlockedActions, &tpl.ExpirationDays, &tpl.NotesTemplate,
		&tpl.IsDefault, &tpl.CreatedBy, &tpl.CreatedAt, &tpl.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("legalhold: template %s not found", templateID)
		}
		return nil, fmt.Errorf("legalhold: get template: %w", err)
	}
	return tpl, nil
}

// ListTemplates returns all hold templates, default first then alphabetically.
func (s *Service) ListTemplates(ctx context.Context, pool *pgxpool.Pool) ([]*domain.HoldTemplate, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, COALESCE(description, ''), provider_glob, blocked_actions, expiration_days, COALESCE(notes_template, ''), is_default, created_by, created_at, updated_at
		FROM hold_templates
		ORDER BY is_default DESC, name ASC
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
			&tpl.IsDefault, &tpl.CreatedBy, &tpl.CreatedAt, &tpl.UpdatedAt,
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
	IsDefault      bool
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

	if params.BlockedActions == nil {
		params.BlockedActions = []string{}
	}
	if params.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE hold_templates SET is_default = false WHERE is_default = true AND id <> $1`, templateID); err != nil {
			return nil, fmt.Errorf("legalhold: clear default template: %w", err)
		}
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
		    is_default      = $8,
		    updated_at      = now()
		WHERE id = $1
		RETURNING id, name, COALESCE(description, ''), provider_glob, blocked_actions, expiration_days, COALESCE(notes_template, ''), is_default, created_by, created_at, updated_at
	`,
		templateID,
		params.Name,
		nullText(params.Description),
		params.ProviderGlob,
		params.BlockedActions,
		nullText(params.NotesTemplate),
		params.ExpirationDays,
		params.IsDefault,
	).Scan(
		&tpl.ID, &tpl.Name, &tpl.Description, &tpl.ProviderGlob,
		&tpl.BlockedActions, &tpl.ExpirationDays, &tpl.NotesTemplate,
		&tpl.IsDefault, &tpl.CreatedBy, &tpl.CreatedAt, &tpl.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("legalhold: template %s not found", templateID)
		}
		return nil, fmt.Errorf("legalhold: update template: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHoldTemplate, templateID, domain.EventHoldTemplateUpdated, actor, map[string]any{
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

	if err := s.appendEvent(ctx, tx, domain.AggregateHoldTemplate, templateID, domain.EventHoldTemplateDeleted, actor, map[string]any{
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
