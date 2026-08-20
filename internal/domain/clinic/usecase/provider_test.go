package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/domain/clinic/model"
	"librevita.org/internal/domain/clinic/usecase"
	mocks "librevita.org/tests/mocks/domain/clinic/model"
)

func TestClockProviderReadsClinicZone(t *testing.T) {
	repoMock := mocks.NewMockRepository(t)
	repoMock.EXPECT().First(context.Background()).Return(&model.Clinic{
		ID:       uuid.MustParse("01990000-0000-7000-8000-000000000001"),
		Timezone: "America/New_York",
	}, nil).Once()

	clock, err := usecase.NewClockProvider(repoMock).Clock(context.Background())
	require.NoError(t, err)

	utc := time.Date(2026, 8, 6, 21, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026-08-06 17:04", clock.FormatUI(utc))
}

func TestClockProviderFallsBackBeforeOnboarding(t *testing.T) {
	repoMock := mocks.NewMockRepository(t)
	repoMock.EXPECT().First(context.Background()).Return(nil, nil).Once()

	clock, err := usecase.NewClockProvider(repoMock).Clock(context.Background())
	require.NoError(t, err)

	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026-08-06 15:04", clock.FormatUI(utc))
}

func TestClockProviderRefreshesAfterTTL(t *testing.T) {
	repoMock := mocks.NewMockRepository(t)
	repoMock.EXPECT().First(context.Background()).Return(&model.Clinic{
		ID:       uuid.MustParse("01990000-0000-7000-8000-000000000001"),
		Timezone: "America/New_York",
	}, nil).Once()

	provider := usecase.NewClockProvider(repoMock)
	clock, err := provider.Clock(context.Background())
	require.NoError(t, err)

	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026-08-06 14:04", clock.FormatUI(utc))

	// Second call within TTL should use cached result and not call First again
	clockCached, err := provider.Clock(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2026-08-06 14:04", clockCached.FormatUI(utc))

	// New provider instance fetches fresh data from repo
	repoMock2 := mocks.NewMockRepository(t)
	repoMock2.EXPECT().First(context.Background()).Return(&model.Clinic{
		ID:       uuid.MustParse("01990000-0000-7000-8000-000000000001"),
		Timezone: "Asia/Tokyo",
	}, nil).Once()

	providerNew := usecase.NewClockProvider(repoMock2)
	clockRefreshed, err := providerNew.Clock(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2026-08-07 03:04", clockRefreshed.FormatUI(utc))
}

func TestClockProviderClinicIDAndProfile(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	clinic := &model.Clinic{
		ID:       clinicID,
		Name:     "Test Clinic",
		Timezone: "America/Sao_Paulo",
	}

	repoMock := mocks.NewMockRepository(t)
	repoMock.EXPECT().First(context.Background()).Return(clinic, nil).Once()

	provider := usecase.NewClockProvider(repoMock)
	id, err := provider.ClinicID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, clinicID.String(), id)

	prof, err := provider.Profile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, clinic, prof)

	// ClockFor with specific timezone
	userClock, err := provider.ClockFor(context.Background(), "Asia/Tokyo")
	require.NoError(t, err)
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026-08-07 03:04", userClock.FormatUI(utc))
}
