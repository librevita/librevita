package audit

import (
	"context"
	"fmt"
	"time"

	"librevita.org/ent"
	"librevita.org/ent/auditlog"
)

type auditRepository struct {
	client *ent.Client
}

// NewAuditRepository creates an audit repository adapter.
func NewAuditRepository(client *ent.Client) Repository {
	return &auditRepository{client: client}
}

func (r *auditRepository) Recent(ctx context.Context, limit int, before int64) ([]EventRow, error) {
	query := r.client.AuditLog.Query()
	if before > 0 {
		query = query.Where(auditlog.IDLT(int(before)))
	}
	rows, err := query.
		Order(ent.Desc(auditlog.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit: recent: %w", err)
	}

	out := make([]EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, EventRow{
			ID:         int64(row.ID),
			CreatedAt:  row.CreatedAt,
			ActorID:    row.ActorID,
			ActorEmail: row.ActorEmail,
			Action:     row.Action,
			Resource:   row.Resource,
			Result:     string(row.Result),
			Detail:     row.Detail,
		})
	}
	return out, nil
}

func (r *auditRepository) ForResource(ctx context.Context, resource string, limit int) ([]EventRow, error) {
	rows, err := r.client.AuditLog.Query().
		Where(auditlog.ResourceEQ(resource)).
		Order(ent.Desc(auditlog.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit: for resource: %w", err)
	}

	out := make([]EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, EventRow{
			ID:         int64(row.ID),
			CreatedAt:  row.CreatedAt,
			ActorID:    row.ActorID,
			ActorEmail: row.ActorEmail,
			Action:     row.Action,
			Resource:   row.Resource,
			Result:     string(row.Result),
			Detail:     row.Detail,
		})
	}
	return out, nil
}

func (r *auditRepository) LastSignature(ctx context.Context) (string, error) {
	last, err := r.client.AuditLog.Query().
		Order(ent.Desc(auditlog.FieldID)).
		First(ctx)
	if ent.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("audit: last signature: %w", err)
	}
	return last.Signature, nil
}

func (r *auditRepository) Record(ctx context.Context, ev Event, createdAt time.Time, signature string) error {
	create := r.client.AuditLog.Create().
		SetAction(ev.Action).
		SetResource(ev.Resource).
		SetResourceName(ev.ResourceName).
		SetResult(auditlog.Result(ev.Result)).
		SetActorName(ev.ActorName).
		SetActorRole(ev.ActorRole).
		SetUserAgent(ev.UserAgent).
		SetSignature(signature).
		SetCreatedAt(createdAt)

	if ev.ActorID != "" {
		create.SetActorID(ev.ActorID)
	}
	if ev.ActorMail != "" {
		create.SetActorEmail(ev.ActorMail)
	}
	if ev.IP != "" {
		create.SetIP(ev.IP)
	}
	if ev.RequestID != "" {
		create.SetRequestID(ev.RequestID)
	}
	if ev.Detail != "" {
		create.SetDetail(ev.Detail)
	}

	_, err := create.Save(ctx)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

func (r *auditRepository) All(ctx context.Context) ([]StoredEntry, error) {
	rows, err := r.client.AuditLog.Query().
		Order(ent.Asc(auditlog.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit: all: %w", err)
	}

	out := make([]StoredEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, StoredEntry{
			ID:           int64(row.ID),
			CreatedAt:    row.CreatedAt,
			ActorID:      row.ActorID,
			ActorEmail:   row.ActorEmail,
			ActorName:    row.ActorName,
			ActorRole:    row.ActorRole,
			UserAgent:    row.UserAgent,
			Action:       row.Action,
			Resource:     row.Resource,
			ResourceName: row.ResourceName,
			Result:       string(row.Result),
			IP:           row.IP,
			RequestID:    row.RequestID,
			Detail:       row.Detail,
			Signature:    row.Signature,
		})
	}
	return out, nil
}
