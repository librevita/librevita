package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/domain/clinic/model"
	"librevita.org/internal/domain/clinic/usecase"
	mocks "librevita.org/tests/mocks/domain/clinic/model"
)

func TestClockProviderReadsClinicZone(t *testing.T) {
	repoMock := mocks.NewMockRepository(t)
	ctx := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{
		ID:       uuid.MustParse("01990000-0000-7000-8000-000000000001"),
		Timezone: "America/New_York",
	})

	clock, err := usecase.NewClockProvider(repoMock).Clock(ctx)
	require.NoError(t, err)

	utc := time.Date(2026, 8, 6, 21, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026-08-06 17:04", clock.FormatUI(utc))
}

func TestClockProviderFallsBackBeforeOnboarding(t *testing.T) {
	repoMock := mocks.NewMockRepository(t)

	clock, err := usecase.NewClockProvider(repoMock).Clock(context.Background())
	require.NoError(t, err)

	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026-08-06 15:04", clock.FormatUI(utc))
}

func TestClockProviderRefreshesAfterTTL(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	ctx := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{
		ID:       clinicID,
		Timezone: "America/New_York",
		Name:     "NY",
	})

	repoMock := mocks.NewMockRepository(t)
	repoMock.EXPECT().GetByID(ctx, clinicID).Return(&model.Clinic{
		ID:       clinicID,
		Timezone: "America/New_York",
		Name:     "NY",
	}, nil).Once()

	provider := usecase.NewClockProvider(repoMock)
	prof, err := provider.Profile(ctx)
	require.NoError(t, err)
	assert.Equal(t, "America/New_York", prof.Timezone)

	profCached, err := provider.Profile(ctx)
	require.NoError(t, err)
	assert.Equal(t, "NY", profCached.Name)

	repoMock2 := mocks.NewMockRepository(t)
	repoMock2.EXPECT().GetByID(ctx, clinicID).Return(&model.Clinic{
		ID:       clinicID,
		Timezone: "Asia/Tokyo",
		Name:     "Tokyo",
	}, nil).Once()

	providerNew := usecase.NewClockProvider(repoMock2)
	profRefreshed, err := providerNew.Profile(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Asia/Tokyo", profRefreshed.Timezone)
}

func TestClockProviderClinicIDAndProfile(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	clinic := &model.Clinic{
		ID:       clinicID,
		Name:     "Test Clinic",
		Timezone: "America/Sao_Paulo",
	}
	ctx := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{
		ID:       clinicID,
		Name:     clinic.Name,
		Timezone: clinic.Timezone,
	})

	repoMock := mocks.NewMockRepository(t)
	repoMock.EXPECT().GetByID(ctx, clinicID).Return(clinic, nil).Once()

	provider := usecase.NewClockProvider(repoMock)
	id, err := provider.ClinicID(ctx)
	require.NoError(t, err)
	assert.Equal(t, clinicID.String(), id)

	prof, err := provider.Profile(ctx)
	require.NoError(t, err)
	assert.Equal(t, clinic, prof)

	userClock, err := provider.ClockFor(ctx, "Asia/Tokyo")
	require.NoError(t, err)
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026-08-07 03:04", userClock.FormatUI(utc))
}
