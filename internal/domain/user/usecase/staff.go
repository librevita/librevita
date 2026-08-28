package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
)

// StaffChange is the JSON payload of a proposed physician profile change.
type StaffChange struct {
	Name        string       `json:"name"`
	Email       string       `json:"email"`
	Specialties []string     `json:"specialties"`
	Previous    *StaffChange `json:"previous,omitempty"`
}

// ListPhysiciansPage returns one page of clinical staff accounts with their specialties joined as a comma-separated string, plus the total.
func (s *Service) ListPhysiciansPage(ctx context.Context, limit, offset int) ([]ListPhysiciansPageRow, int64, error) {
	return s.userRepo.ListPhysiciansPage(ctx, limit, offset)
}

// CreateStaffChangeRequest records a receptionist's proposal to change a physician's profile.
func (s *Service) CreateStaffChangeRequest(ctx context.Context, userID, requestedBy string, change StaffChange) (*StaffChangeRequest, error) {
	change.Name = strings.TrimSpace(change.Name)
	change.Email = normalizeEmail(change.Email)
	if change.Name == "" {
		return nil, &ValidationError{Msg: "display name is required"}
	}
	if err := validateEmail(change.Email); err != nil {
		return nil, err
	}
	if err := validateSpecialtyIDs(change.Specialties); err != nil {
		return nil, err
	}

	uUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid user id")
	}
	reqUUID, err := uuid.Parse(requestedBy)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid requester id")
	}

	current, err := s.userRepo.GetByID(ctx, uUUID)
	if err != nil {
		return nil, err
	}

	userSpecs, err := s.specialtyRepo.ListByUser(ctx, uUUID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: list user specialties")
	}

	previous := &StaffChange{Name: current.DisplayName, Email: current.Email}
	for _, sp := range userSpecs {
		previous.Specialties = append(previous.Specialties, sp.ID.String())
	}
	change.Previous = previous

	if change.Email != current.Email {
		other, err := s.userRepo.GetByEmail(ctx, change.Email)
		if err == nil && other.ID != uUUID {
			return nil, ErrEmailInUse
		}
		if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, errors.Wrap(err, "usecase: check email")
		}
	}

	payload, err := json.Marshal(change)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: encode staff change")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, errors.Wrap(err, "usecase: generate request id")
	}

	req, err := s.staffReqRepo.Create(ctx, &StaffChangeRequest{
		ID:          id,
		UserID:      uUUID,
		RequestedBy: reqUUID,
		Changes:     string(payload),
	})
	if err != nil {
		return nil, errors.Wrap(err, "usecase: create staff change request")
	}
	return req, nil
}

// ListMyStaffChangeRequests returns every request made by the given requester.
func (s *Service) ListMyStaffChangeRequests(ctx context.Context, requesterID string) ([]ListStaffChangeRequestsByRequesterRow, error) {
	reqUUID, err := uuid.Parse(requesterID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid requester id")
	}
	return s.staffReqRepo.ListByRequester(ctx, reqUUID, 50)
}

// ListStaffChangeRequestsFiltered returns the change requests filtered by status and search term.
func (s *Service) ListStaffChangeRequestsFiltered(ctx context.Context, status, q string, limit, offset int) ([]ListStaffChangeRequestsFilteredRow, int64, error) {
	return s.staffReqRepo.ListFiltered(ctx, status, q, limit, offset)
}

// ApproveStaffChangeRequest applies the proposed changes to the physician account and marks the request approved.
func (s *Service) ApproveStaffChangeRequest(ctx context.Context, id, decidedBy string) error {
	reqUUID, err := uuid.Parse(id)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid request id")
	}
	deciderUUID, err := uuid.Parse(decidedBy)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid decider id")
	}

	req, err := s.staffReqRepo.GetByID(ctx, reqUUID)
	if err != nil {
		return err
	}
	if StaffRequestStatus(req.Status) != StaffRequestPending {
		return ErrRequestNotPending
	}

	var change StaffChange
	if err := json.Unmarshal([]byte(req.Changes), &change); err != nil {
		return errors.Wrap(err, "usecase: decode staff change")
	}
	if err := validateSpecialtyIDs(change.Specialties); err != nil {
		return err
	}

	var spUUIDs []uuid.UUID
	for _, spID := range change.Specialties {
		if spID != "" {
			spUUIDs = append(spUUIDs, uuid.MustParse(spID))
		}
	}

	return s.userRepo.ApplyApprovedStaffChange(ctx, reqUUID, req.UserID, deciderUUID, change.Name, change.Email, spUUIDs)
}

// RejectStaffChangeRequest marks the request rejected with a note.
func (s *Service) RejectStaffChangeRequest(ctx context.Context, id, decidedBy, note string) error {
	reqUUID, err := uuid.Parse(id)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid request id")
	}
	deciderUUID, err := uuid.Parse(decidedBy)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid decider id")
	}

	req, err := s.staffReqRepo.GetByID(ctx, reqUUID)
	if err != nil {
		return err
	}
	if StaffRequestStatus(req.Status) != StaffRequestPending {
		return ErrRequestNotPending
	}

	return s.staffReqRepo.Reject(ctx, reqUUID, deciderUUID, strings.TrimSpace(note))
}
