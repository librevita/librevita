package audit

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/auditlog"
	"librevita.org/pkg/ident"
)

type auditRepository struct {
	client *record.Client
}

// NewAuditRepository creates an audit repository adapter.
func NewAuditRepository(client *record.Client) Repository {
	return &auditRepository{client: client}
}

func (r *auditRepository) Recent(ctx context.Context, limit int, before int64) ([]EventRow, error) {
	query := r.client.AuditLog.Query()
	if id, ok := clinicctx.ClinicID(ctx); ok {
		query = query.Where(auditlog.ClinicIDEQ(id))
	}
	if before > 0 {
		query = query.Where(auditlog.IDLT(int(before)))
	}
	rows, err := query.
		Order(record.Desc(auditlog.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "audit: recent")
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
	query := r.client.AuditLog.Query().Where(auditlog.ResourceEQ(resource))
	if id, ok := clinicctx.ClinicID(ctx); ok {
		query = query.Where(auditlog.ClinicIDEQ(id))
	}
	rows, err := query.
		Order(record.Desc(auditlog.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "audit: for resource")
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
	ctx = clinicctx.WithSkipIsolation(ctx)
	last, err := r.client.AuditLog.Query().
		Order(record.Desc(auditlog.FieldID)).
		First(ctx)
	if record.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", errors.Wrap(err, "audit: last signature")
	}
	return last.Signature, nil
}

func (r *auditRepository) Record(ctx context.Context, ev Event, createdAt time.Time, signature string) error {
	ctx = clinicctx.WithSkipIsolation(ctx)
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

	if ev.ClinicID != "" {
		if cid, err := ident.ParseClinic(ev.ClinicID); err == nil {
			create.SetClinicID(cid)
		}
	}

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
		return errors.Wrap(err, "audit: insert")
	}
	return nil
}

func (r *auditRepository) All(ctx context.Context) ([]StoredEntry, error) {
	ctx = clinicctx.WithSkipIsolation(ctx)
	rows, err := r.client.AuditLog.Query().
		Order(record.Asc(auditlog.FieldID)).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "audit: all")
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
			ClinicID:     uuidString(row.ClinicID),
		})
	}
	return out, nil
}

func uuidString(cid *ident.ClinicID) *string {
	if cid == nil {
		return nil
	}
	s := cid.String()
	return &s
}
