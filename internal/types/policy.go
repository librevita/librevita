package types

// PolicyOrigin records who introduced a policy version. It mirrors the
// CHECK constraint on policy_versions.origin (see
// db/migrations/00005_policy.sql).
type PolicyOrigin string

const (
	PolicyOriginSeed   PolicyOrigin = "seed"
	PolicyOriginAdmin  PolicyOrigin = "admin"
	PolicyOriginSystem PolicyOrigin = "system"
)

// Valid reports whether o is one of the options the database CHECK
// constraint accepts.
func (o PolicyOrigin) Valid() bool {
	switch o {
	case PolicyOriginSeed, PolicyOriginAdmin, PolicyOriginSystem:
		return true
	}
	return false
}

// String returns the stored representation of o.
func (o PolicyOrigin) String() string {
	return string(o)
}

// ParsePolicyOrigin converts a stored value back to the enum. ok is
// false when the value is not one of the CHECK options.
func ParsePolicyOrigin(s string) (PolicyOrigin, bool) {
	origin := PolicyOrigin(s)
	return origin, origin.Valid()
}
