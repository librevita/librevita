package clinicctx_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/pkg/ident"
)

func TestClinicContext(t *testing.T) {
	ctx := context.Background()

	// 1. Initially no clinic attached
	c, ok := clinicctx.FromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, c)

	cid, ok := clinicctx.ClinicID(ctx)
	assert.False(t, ok)
	assert.True(t, cid.IsZero())

	_, err := clinicctx.MustClinicID(ctx)
	assert.ErrorIs(t, err, clinicctx.ErrMissingClinic)

	// 2. Attach Clinic
	testID := ident.New[ident.ClinicID]()
	now := time.Now()
	clinic := &clinicctx.Clinic{
		ID:          testID,
		Slug:        "clinica-teste",
		Name:        "Clínica Teste",
		Timezone:    "America/Sao_Paulo",
		OnboardedAt: &now,
	}

	ctxWithClinic := clinicctx.WithClinic(ctx, clinic)
	cGot, ok := clinicctx.FromContext(ctxWithClinic)
	require.True(t, ok)
	assert.Equal(t, testID, cGot.ID)
	assert.Equal(t, "clinica-teste", cGot.Slug)

	cidGot, ok := clinicctx.ClinicID(ctxWithClinic)
	assert.True(t, ok)
	assert.Equal(t, testID, cidGot)

	cidMust, err := clinicctx.MustClinicID(ctxWithClinic)
	require.NoError(t, err)
	assert.Equal(t, testID, cidMust)

	// 3. Skip Isolation
	assert.False(t, clinicctx.IsolationSkipped(ctx))
	ctxSkip := clinicctx.WithSkipIsolation(ctx)
	assert.True(t, clinicctx.IsolationSkipped(ctxSkip))

	// 4. Apex
	assert.False(t, clinicctx.IsApex(ctx))
	ctxApex := clinicctx.WithApex(ctx)
	assert.True(t, clinicctx.IsApex(ctxApex))

	// 5. Reserved slugs
	assert.True(t, clinicctx.IsReservedSlug("www"))
	assert.True(t, clinicctx.IsReservedSlug("admin"))
	assert.True(t, clinicctx.IsReservedSlug("app"))
	assert.True(t, clinicctx.IsReservedSlug("api"))
	assert.True(t, clinicctx.IsReservedSlug("mail"))
	assert.False(t, clinicctx.IsReservedSlug("clinica"))

	// 6. TestClinic helper
	ctxTest := clinicctx.WithTestClinic(ctx)
	cTest, ok := clinicctx.FromContext(ctxTest)
	require.True(t, ok)
	assert.Equal(t, clinicctx.TestClinicID, cTest.ID)
	assert.Equal(t, "test", cTest.Slug)
}
