package usecase

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"librevita.org/internal/domain/user/repository"
)

// Staff request errors.
var (
	ErrRequestNotFound   = errors.New("usecase: staff change request not found")
	ErrRequestNotPending = errors.New("usecase: staff change request is not pending")
	ErrEmailInUse        = errors.New("usecase: that email already belongs to another account")
)

// Staff change request states.
const (
	RequestPending  = "pending"
	RequestApproved = "approved"
	RequestRejected = "rejected"
)

// StaffChange is the JSON payload of a proposed physician profile change.
type StaffChange struct {
	Name        string       `json:"name"`
	Email       string       `json:"email"`
	Specialties []string     `json:"specialties"`
	Previous    *StaffChange `json:"previous,omitempty"`
}

// ListPhysicians returns the physician accounts with their specialties
// joined as a comma-separated string.
func (s *Service) ListPhysicians(ctx context.Context) ([]repository.ListPhysiciansRow, error) {
	rows, err := s.users.ListPhysicians(ctx)
	if err != nil {
		return nil, fmt.Errorf("usecase: list physicians: %w", err)
	}
	return rows, nil
}

// CreateStaffChangeRequest records a receptionist's proposal to change a
// physician's profile. The request stays pending until an admin decides.
func (s *Service) CreateStaffChangeRequest(ctx context.Context, userID, requestedBy string, change StaffChange) (*repository.StaffChangeRequest, error) {
	change.Name = strings.TrimSpace(change.Name)
	change.Email = normalizeEmail(change.Email)
	if change.Name == "" {
		return nil, &ValidationError{Msg: "display name is required"}
	}
	if err := validateEmail(change.Email); err != nil {
		return nil, err
	}

	// Reject proposals that would collide with another account's email.
	current, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("usecase: load staff user: %w", err)
	}
	// Snapshot the profile at request time so the diff stays readable
	// after the change was applied or rejected.
	previous := &StaffChange{Name: current.DisplayName, Email: current.Email}
	assigned, err := s.users.ListUserSpecialties(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("usecase: load staff specialties: %w", err)
	}
	for _, sp := range assigned {
		previous.Specialties = append(previous.Specialties, sp.ID)
	}
	change.Previous = previous
	if change.Email != current.Email {
		other, err := s.users.GetUserByEmail(ctx, change.Email)
		if err == nil && other.ID != userID {
			return nil, ErrEmailInUse
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("usecase: check email: %w", err)
		}
	}

	payload, err := json.Marshal(change)
	if err != nil {
		return nil, fmt.Errorf("usecase: encode staff change: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("usecase: generate request id: %w", err)
	}
	req, err := s.users.CreateStaffChangeRequest(ctx, repository.CreateStaffChangeRequestParams{
		ID: id.String(), UserID: userID, RequestedBy: requestedBy, Changes: string(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("usecase: create staff change request: %w", err)
	}
	return &req, nil
}

// ListMyStaffChangeRequests returns every request made by the given
// requester, newest first, with the current status.
func (s *Service) ListMyStaffChangeRequests(ctx context.Context, requesterID string) ([]repository.ListStaffChangeRequestsByRequesterRow, error) {
	rows, err := s.users.ListStaffChangeRequestsByRequester(ctx, repository.ListStaffChangeRequestsByRequesterParams{
		RequestedBy: requesterID, Limit: 50,
	})
	if err != nil {
		return nil, fmt.Errorf("usecase: list my staff change requests: %w", err)
	}
	return rows, nil
}

// ListStaffChangeRequests returns the requests in the given status,
// newest first, joined with the target and requester emails.
func (s *Service) ListStaffChangeRequests(ctx context.Context, status string) ([]repository.ListStaffChangeRequestsRow, error) {
	rows, err := s.users.ListStaffChangeRequests(ctx, repository.ListStaffChangeRequestsParams{
		Status: status, Limit: 50,
	})
	if err != nil {
		return nil, fmt.Errorf("usecase: list staff change requests: %w", err)
	}
	return rows, nil
}

// ApproveStaffChangeRequest applies the proposed changes to the
// physician account and marks the request approved. The approval is a
// single transaction, so the applied state never diverges from the
// request status.
func (s *Service) ApproveStaffChangeRequest(ctx context.Context, id, decidedBy string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("usecase: begin staff change tx: %w", err)
	}
	queries := repository.New(tx)
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	req, err := queries.GetStaffChangeRequest(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRequestNotFound
		}
		return fmt.Errorf("usecase: load staff change request: %w", err)
	}
	if req.Status != RequestPending {
		return ErrRequestNotPending
	}
	var change StaffChange
	if err := json.Unmarshal([]byte(req.Changes), &change); err != nil {
		return fmt.Errorf("usecase: decode staff change: %w", err)
	}

	active := int64(1)
	user, err := queries.GetUserByID(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("usecase: load staff user: %w", err)
	}
	if user.Active == 0 {
		active = 0
	}
	if _, err := queries.UpdateUser(ctx, repository.UpdateUserParams{
		ID:          req.UserID,
		Email:       change.Email,
		DisplayName: change.Name,
		RoleID:      user.RoleID,
		Active:      active,
	}); err != nil {
		return ErrEmailInUse
	}
	if err := queries.ClearUserSpecialties(ctx, req.UserID); err != nil {
		return fmt.Errorf("usecase: clear staff specialties: %w", err)
	}
	for _, spID := range change.Specialties {
		if spID == "" {
			continue
		}
		if err := queries.AddUserSpecialty(ctx, repository.AddUserSpecialtyParams{
			UserID: req.UserID, SpecialtyID: spID,
		}); err != nil {
			return fmt.Errorf("usecase: add staff specialty: %w", err)
		}
	}
	if err := queries.DecideStaffChangeRequest(ctx, repository.DecideStaffChangeRequestParams{
		Status: RequestApproved, DecisionNote: sql.NullString{}, DecidedBy: sql.NullString{String: decidedBy, Valid: true}, ID: id,
	}); err != nil {
		return fmt.Errorf("usecase: approve staff change request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("usecase: commit staff change tx: %w", err)
	}
	committed = true
	return nil
}

// RejectStaffChangeRequest marks the request rejected with a note.
func (s *Service) RejectStaffChangeRequest(ctx context.Context, id, decidedBy, note string) error {
	req, err := s.users.GetStaffChangeRequest(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRequestNotFound
		}
		return fmt.Errorf("usecase: load staff change request: %w", err)
	}
	if req.Status != RequestPending {
		return ErrRequestNotPending
	}
	note = strings.TrimSpace(note)
	if err := s.users.DecideStaffChangeRequest(ctx, repository.DecideStaffChangeRequestParams{
		Status: RequestRejected, DecisionNote: sql.NullString{String: note, Valid: note != ""},
		DecidedBy: sql.NullString{String: decidedBy, Valid: true}, ID: id,
	}); err != nil {
		return fmt.Errorf("usecase: reject staff change request: %w", err)
	}
	return nil
}
