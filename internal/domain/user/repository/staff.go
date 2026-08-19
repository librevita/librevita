package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/staffchangerequest"
	"librevita.org/ent/user"
	usermodel "librevita.org/internal/domain/user/model"
	"librevita.org/internal/types"
)

type staffRequestRepository struct {
	client *ent.Client
}

// NewStaffRequestRepository creates a staff change request repository adapter.
func NewStaffRequestRepository(client *ent.Client) usermodel.StaffRequestRepository {
	return &staffRequestRepository{client: client}
}

func (r *staffRequestRepository) Create(ctx context.Context, req *usermodel.StaffChangeRequest) (*usermodel.StaffChangeRequest, error) {
	saved, err := r.client.StaffChangeRequest.Create().
		SetID(req.ID).
		SetUserID(req.UserID).
		SetRequestedBy(req.RequestedBy).
		SetChanges(req.Changes).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("staff request repository: create: %w", err)
	}

	return toStaffChangeRequestDomain(saved), nil
}

func (r *staffRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*usermodel.StaffChangeRequest, error) {
	req, err := r.client.StaffChangeRequest.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, usermodel.ErrRequestNotFound
		}
		return nil, fmt.Errorf("staff request repository: get by id: %w", err)
	}
	return toStaffChangeRequestDomain(req), nil
}

func (r *staffRequestRepository) ListByRequester(ctx context.Context, requesterID uuid.UUID, limit int) ([]usermodel.ListStaffChangeRequestsByRequesterRow, error) {
	requests, err := r.client.StaffChangeRequest.Query().
		Where(staffchangerequest.RequestedByEQ(requesterID)).
		WithUser().
		Order(ent.Desc(staffchangerequest.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("staff request repository: list by requester: %w", err)
	}

	rows := make([]usermodel.ListStaffChangeRequestsByRequesterRow, 0, len(requests))
	for _, req := range requests {
		staffName, staffEmail := "", ""
		if req.Edges.User != nil {
			staffName = req.Edges.User.DisplayName
			staffEmail = req.Edges.User.Email
		}
		rows = append(rows, usermodel.ListStaffChangeRequestsByRequesterRow{
			ID:           req.ID,
			UserID:       req.UserID,
			StaffName:    staffName,
			StaffEmail:   staffEmail,
			RequestedBy:  req.RequestedBy,
			Status:       req.Status,
			Changes:      req.Changes,
			DecisionNote: req.DecisionNote,
			CreatedAt:    req.CreatedAt,
			DecidedAt:    req.DecidedAt,
		})
	}
	return rows, nil
}

func (r *staffRequestRepository) ListFiltered(ctx context.Context, status, q string, limit, offset int) ([]usermodel.ListStaffChangeRequestsFilteredRow, int64, error) {
	query := r.client.StaffChangeRequest.Query().
		WithUser().
		WithRequester().
		WithDecider().
		Order(ent.Desc(staffchangerequest.FieldCreatedAt))

	if status != "" {
		query = query.Where(staffchangerequest.StatusEQ(status))
	}
	if q != "" {
		query = query.Where(
			staffchangerequest.Or(
				staffchangerequest.HasUserWith(user.DisplayNameContainsFold(q)),
				staffchangerequest.HasUserWith(user.EmailContainsFold(q)),
				staffchangerequest.HasRequesterWith(user.DisplayNameContainsFold(q)),
			),
		)
	}

	requests, err := query.Limit(limit).Offset(offset).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("staff request repository: list filtered: %w", err)
	}

	countQuery := r.client.StaffChangeRequest.Query()
	if status != "" {
		countQuery = countQuery.Where(staffchangerequest.StatusEQ(status))
	}
	if q != "" {
		countQuery = countQuery.Where(
			staffchangerequest.Or(
				staffchangerequest.HasUserWith(user.DisplayNameContainsFold(q)),
				staffchangerequest.HasUserWith(user.EmailContainsFold(q)),
				staffchangerequest.HasRequesterWith(user.DisplayNameContainsFold(q)),
			),
		)
	}
	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("staff request repository: count filtered: %w", err)
	}

	rows := make([]usermodel.ListStaffChangeRequestsFilteredRow, 0, len(requests))
	for _, req := range requests {
		staffName, staffEmail := "", ""
		if req.Edges.User != nil {
			staffName = req.Edges.User.DisplayName
			staffEmail = req.Edges.User.Email
		}
		requesterName := ""
		if req.Edges.Requester != nil {
			requesterName = req.Edges.Requester.DisplayName
		}
		var deciderName *string
		if req.Edges.Decider != nil {
			dn := req.Edges.Decider.DisplayName
			deciderName = &dn
		}
		rows = append(rows, usermodel.ListStaffChangeRequestsFilteredRow{
			ID:            req.ID,
			UserID:        req.UserID,
			StaffName:     staffName,
			StaffEmail:    staffEmail,
			RequestedBy:   req.RequestedBy,
			RequesterName: requesterName,
			Status:        req.Status,
			Changes:       req.Changes,
			DecisionNote:  req.DecisionNote,
			CreatedAt:     req.CreatedAt,
			DecidedAt:     req.DecidedAt,
			DecidedBy:     req.DecidedBy,
			DeciderName:   deciderName,
		})
	}
	return rows, int64(total), nil
}

func (r *staffRequestRepository) Reject(ctx context.Context, id, deciderID uuid.UUID, note string) error {
	decidedAt := time.Now().UTC()
	update := r.client.StaffChangeRequest.UpdateOneID(id).
		SetStatus(types.StaffRequestRejected.String()).
		SetDecidedAt(decidedAt).
		SetDecidedBy(deciderID)
	if note != "" {
		update.SetDecisionNote(note)
	}
	if err := update.Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return usermodel.ErrRequestNotFound
		}
		return fmt.Errorf("staff request repository: reject: %w", err)
	}
	return nil
}

func toStaffChangeRequestDomain(req *ent.StaffChangeRequest) *usermodel.StaffChangeRequest {
	if req == nil {
		return nil
	}
	return &usermodel.StaffChangeRequest{
		ID:           req.ID,
		UserID:       req.UserID,
		RequestedBy:  req.RequestedBy,
		Status:       req.Status,
		Changes:      req.Changes,
		DecisionNote: req.DecisionNote,
		CreatedAt:    req.CreatedAt,
		DecidedAt:    req.DecidedAt,
		DecidedBy:    req.DecidedBy,
	}
}
