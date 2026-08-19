package model

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Clinic is the domain model representing a clinic profile.
type Clinic struct {
	ID         uuid.UUID
	Name       string
	TaxID      string
	Phone      string
	Email      string
	Street     string
	City       string
	State      string
	PostalCode string
	Country    string
	Timezone   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Repository defines the storage contract for clinic data.
type Repository interface {
	First(ctx context.Context) (*Clinic, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Clinic, error)
}
