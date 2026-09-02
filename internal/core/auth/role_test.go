package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/auth"
)

func TestRole(t *testing.T) {
	roles := []auth.Role{
		auth.RoleAdmin,
		auth.RolePhysician,
		auth.RoleReceptionist,
		auth.RolePatient,
	}

	for _, r := range roles {
		assert.True(t, r.Valid())
		assert.NotEmpty(t, r.String())

		parsed, err := auth.ParseRole(string(r))
		assert.NoError(t, err)
		assert.Equal(t, r, parsed)
	}

	invalid := auth.Role("superman")
	assert.False(t, invalid.Valid())

	_, err := auth.ParseRole("superman")
	assert.Error(t, err)
}
