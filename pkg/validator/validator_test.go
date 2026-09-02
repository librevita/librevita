package validator_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/pkg/validator"
)

func TestValidatorBasicFlow(t *testing.T) {
	v := validator.New()
	assert.True(t, v.Valid())
	assert.False(t, v.HasErrors())
	assert.NoError(t, v.Err())
	assert.Empty(t, v.FirstError())

	v.Check(true, "name", "name is required")
	assert.True(t, v.Valid())

	v.Check(false, "name", "name is required")
	assert.False(t, v.Valid())
	assert.True(t, v.HasErrors())
	assert.Equal(t, "name is required", v.FieldError("name"))
	assert.Equal(t, "name is required", v.FirstError())

	// First error wins
	v.Check(false, "name", "name is too short")
	assert.Equal(t, "name is required", v.FieldError("name"))

	v.Check(false, "email", "invalid email")
	assert.Equal(t, "invalid email", v.FieldError("email"))

	err := v.Err()
	require.Error(t, err)

	var valErr *validator.ValidationError
	require.True(t, errors.As(err, &valErr))
	assert.Equal(t, 2, len(valErr.FieldErrors))
	assert.Equal(t, 2, len(valErr.Errors()))
	assert.Contains(t, err.Error(), "name: name is required")
	assert.Contains(t, err.Error(), "email: invalid email")
}

func TestValidatorNonFieldErrors(t *testing.T) {
	v := validator.New()
	v.CheckNonField(true, "general error")
	assert.True(t, v.Valid())

	v.CheckNonField(false, "invalid form state")
	assert.False(t, v.Valid())
	assert.Equal(t, "invalid form state", v.FirstError())
	assert.Equal(t, "invalid form state", v.Err().Error())

	// First error wins for non-field
	v.AddNonFieldError("second error")
	assert.Equal(t, "invalid form state", v.FirstError())

	// AddFieldError with empty field name routes to non-field error
	v2 := validator.New()
	v2.AddError("", "empty field name error")
	assert.Equal(t, "empty field name error", v2.FirstError())
}

func TestI18nFormatting(t *testing.T) {
	t.Run("english default catalog", func(t *testing.T) {
		v := validator.New()
		v.Field("", "display_name", "patient name").Required()
		v.Field("a", "password", "password").Min(8)
		v.Field("toolong", "tag", "tag").Max(3)
		v.Field("bad", "email", "email").Email()
		v.Field("bad", "birth_date", "birth date").DateISO()
		v.Field("bad", "id", "UUID").UUID()

		assert.Equal(t, "patient name is required", v.FieldError("display_name"))
		assert.Equal(t, "password must be at least 8 characters", v.FieldError("password"))
		assert.Equal(t, "tag must be at most 3 characters", v.FieldError("tag"))
		assert.Equal(t, "enter a valid email address", v.FieldError("email"))
		assert.Equal(t, "enter a valid date (YYYY-MM-DD)", v.FieldError("birth_date"))
		assert.Equal(t, "enter a valid UUID", v.FieldError("id"))
	})

	t.Run("portuguese catalog", func(t *testing.T) {
		v := validator.New(validator.WithValidatorLocale("pt-BR"))
		v.Field("", "display_name", "Nome do paciente").Required()
		v.Field("a", "password", "Senha").Min(8)
		v.Field("toolong", "tag", "Tag").Max(3)
		v.Field("bad", "email", "Email").Email()
		v.Field("bad", "birth_date", "Data de nascimento").DateISO()
		v.Field("bad", "id", "UUID").UUID()

		assert.Equal(t, "Nome do paciente é obrigatório", v.FieldError("display_name"))
		assert.Equal(t, "Senha deve ter no mínimo 8 caracteres", v.FieldError("password"))
		assert.Equal(t, "Tag deve ter no máximo 3 caracteres", v.FieldError("tag"))
		assert.Equal(t, "informe um endereço de e-mail válido", v.FieldError("email"))
		assert.Equal(t, "informe uma data válida (AAAA-MM-DD)", v.FieldError("birth_date"))
		assert.Equal(t, "informe um UUID válido", v.FieldError("id"))
	})

	t.Run("from context", func(t *testing.T) {
		ctx := validator.WithLocale(context.Background(), "pt-BR")
		v := validator.FromContext(ctx)
		v.Field("", "name", "Nome").Required()
		assert.Equal(t, "Nome é obrigatório", v.FieldError("name"))
	})

	t.Run("custom message override", func(t *testing.T) {
		v := validator.New()
		v.Field("", "name").Required("por favor informe seu nome")
		assert.Equal(t, "por favor informe seu nome", v.FieldError("name"))
	})
}

func TestFluentBuilder(t *testing.T) {
	t.Run("valid input passes all rules", func(t *testing.T) {
		v := validator.New()
		v.Field("Alice", "name", "name").Required().Between(2, 50)
		v.Field("alice@example.com", "email", "email").Required().Email()
		v.Field("2020-05-15", "birth_date", "birth date").Optional().DateISO()
		v.Field("018d3b8f-3d60-7000-8000-000000000000", "id", "id").Optional().UUID()
		v.Field("active", "status", "status").In([]string{"active", "archived"})
		v.Field("custom", "tag", "tag").Custom(func(s string) bool { return s == "custom" }, "tag.invalid")

		assert.True(t, v.Valid())
		assert.NoError(t, v.Err())
	})

	t.Run("optional field skips subsequent checks if empty", func(t *testing.T) {
		v := validator.New()
		v.Field("", "birth_date", "birth date").Optional().DateISO().Min(10)
		assert.True(t, v.Valid())
		assert.NoError(t, v.Err())
	})

	t.Run("optional field executes checks if non-empty", func(t *testing.T) {
		v := validator.New()
		v.Field("not-a-date", "birth_date", "birth date").Optional().DateISO()
		assert.False(t, v.Valid())
		assert.Equal(t, "enter a valid date (YYYY-MM-DD)", v.FieldError("birth_date"))
		assert.Equal(t, validator.CodeInvalidDate, v.Err().(*validator.ValidationError).FieldError("birth_date").Code)
	})

	t.Run("required field short circuits on failure", func(t *testing.T) {
		v := validator.New()
		v.Field("   ", "name", "name").Required().Max(2)
		assert.False(t, v.Valid())
		assert.Equal(t, "name is required", v.FieldError("name"))
	})
}

func TestRulesRuneCountsAndRules(t *testing.T) {
	str := "São Paulo 🏥"
	assert.True(t, validator.MinRunes(str, 5))
	assert.True(t, validator.MinRunes(str, 11))
	assert.False(t, validator.MinRunes(str, 12))

	assert.True(t, validator.MaxRunes(str, 20))
	assert.True(t, validator.MaxRunes(str, 11))
	assert.False(t, validator.MaxRunes(str, 10))

	assert.True(t, validator.BetweenRunes(str, 5, 15))
	assert.False(t, validator.BetweenRunes(str, 12, 15))

	assert.True(t, validator.ValidEmail("test@example.com"))
	assert.False(t, validator.ValidEmail("invalid-email"))

	digitsOnly := regexp.MustCompile(`^\d+$`)
	assert.True(t, validator.Matches("123456", digitsOnly))
	assert.False(t, validator.Matches("123a56", digitsOnly))

	assert.True(t, validator.ValidUUID("018d3b8f-3d60-7000-8000-000000000000"))
	assert.False(t, validator.ValidUUID("not-a-uuid"))
}

type mockValidatable bool

func (m mockValidatable) Valid() bool { return bool(m) }

func TestValidatable(t *testing.T) {
	v := validator.New()
	v.Validatable(mockValidatable(true), "status", "status")
	assert.True(t, v.Valid())

	v.Validatable(mockValidatable(false), "status", "status")
	assert.False(t, v.Valid())
	assert.Equal(t, "invalid status", v.FieldError("status"))

	v2 := validator.New(validator.WithValidatorLocale("pt-BR"))
	v2.Validatable(mockValidatable(false), "status", "status")
	assert.Equal(t, "status inválido", v2.FieldError("status"))

	v3 := validator.New()
	v3.Validatable(nil, "nil_field", "field")
	assert.False(t, v3.Valid())
	assert.Equal(t, "invalid field", v3.FieldError("nil_field"))
}

func TestBuilderBetweenMatchesInCustom(t *testing.T) {
	v := validator.New()
	v.Field("abc", "code").Between(2, 5)
	v.Field("123", "pin").Matches(regexp.MustCompile(`^\d{3}$`))
	v.Field("admin", "role").In([]string{"admin", "user"})
	v.Field("valid", "custom").Custom(func(s string) bool { return len(s) == 5 }, validator.CodeCustom)
	assert.True(t, v.Valid())

	vFail := validator.New()
	vFail.Field("a", "code").Between(3, 5)
	vFail.Field("abc", "pin").Matches(regexp.MustCompile(`^\d{3}$`))
	vFail.Field("super", "role").In([]string{"admin", "user"})
	vFail.Field("invalid", "custom").Custom(func(s string) bool { return len(s) == 3 }, validator.CodeCustom)
	assert.False(t, vFail.Valid())
	assert.NotEmpty(t, vFail.FieldError("code"))
	assert.NotEmpty(t, vFail.FieldError("pin"))
	assert.NotEmpty(t, vFail.FieldError("role"))
	assert.NotEmpty(t, vFail.FieldError("custom"))
}

func TestGenericField(t *testing.T) {
	v := validator.New()
	validator.FieldValue(v, 42, "number").In([]int{10, 20, 42})
	validator.FieldValue(v, 100, "score").Custom(func(n int) bool { return n >= 50 }, validator.CodeCustom)
	assert.True(t, v.Valid())

	vFail := validator.New()
	validator.FieldValue(vFail, 99, "number").In([]int{10, 20, 42})
	validator.FieldValue(vFail, 30, "score").Custom(func(n int) bool { return n >= 50 }, validator.CodeCustom)
	assert.False(t, vFail.Valid())
	assert.NotEmpty(t, vFail.FieldError("number"))
	assert.NotEmpty(t, vFail.FieldError("score"))

	assert.True(t, validator.In("a", "a", "b", "c"))
	assert.False(t, validator.In("x", "a", "b", "c"))
	assert.True(t, validator.NotIn("x", "a", "b", "c"))
	assert.False(t, validator.NotIn("a", "a", "b", "c"))
}
