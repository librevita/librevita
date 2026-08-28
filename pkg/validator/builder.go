package validator

import (
	"regexp"
	"time"
)

// StringField provides a fluent builder for validating string fields with i18n support.
type StringField struct {
	v       *Validator
	field   string
	label   string
	val     string
	skipped bool
}

// Field starts a fluent validation chain for a string field.
// label is optional; if omitted, the field name is humanized.
func (v *Validator) Field(value, field string, label ...string) *StringField {
	l := HumanizeFieldName(field)
	if len(label) > 0 && label[0] != "" {
		l = label[0]
	}
	return &StringField{
		v:     v,
		field: field,
		label: l,
		val:   value,
	}
}

// Required asserts that the field is not empty or whitespace-only.
func (f *StringField) Required(customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !NotBlank(f.val) {
		msg := FormatDefaultMessage(f.v.locale, CodeRequired, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeRequired,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// Optional marks the field as optional. If the value is blank,
// all subsequent validations in the chain are skipped.
func (f *StringField) Optional() *StringField {
	if f.skipped {
		return f
	}
	if !NotBlank(f.val) {
		f.skipped = true
	}
	return f
}

// Min asserts that the string has at least min unicode runes.
func (f *StringField) Min(min int, customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !MinRunes(f.val, min) {
		params := map[string]any{"min": min}
		msg := FormatDefaultMessage(f.v.locale, CodeMinRunes, f.label, params, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeMinRunes,
			Message: msg,
			Params:  params,
		})
		f.skipped = true
	}
	return f
}

// Max asserts that the string has at most max unicode runes.
func (f *StringField) Max(max int, customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !MaxRunes(f.val, max) {
		params := map[string]any{"max": max}
		msg := FormatDefaultMessage(f.v.locale, CodeMaxRunes, f.label, params, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeMaxRunes,
			Message: msg,
			Params:  params,
		})
		f.skipped = true
	}
	return f
}

// Between asserts that the string's rune count is between min and max inclusive.
func (f *StringField) Between(min, max int, customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !BetweenRunes(f.val, min, max) {
		params := map[string]any{"min": min, "max": max}
		msg := FormatDefaultMessage(f.v.locale, CodeBetweenRunes, f.label, params, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeBetweenRunes,
			Message: msg,
			Params:  params,
		})
		f.skipped = true
	}
	return f
}

// Email asserts that the string is a valid RFC 5322 email with a dotted domain.
func (f *StringField) Email(customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !ValidEmail(f.val) {
		msg := FormatDefaultMessage(f.v.locale, CodeInvalidEmail, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeInvalidEmail,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// DateISO asserts that the string matches the ISO date format YYYY-MM-DD.
func (f *StringField) DateISO(customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if _, err := time.Parse("2006-01-02", f.val); err != nil {
		msg := FormatDefaultMessage(f.v.locale, CodeInvalidDate, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeInvalidDate,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// UUID asserts that the string is a valid UUID.
func (f *StringField) UUID(customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !ValidUUID(f.val) {
		msg := FormatDefaultMessage(f.v.locale, CodeInvalidUUID, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeInvalidUUID,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// Matches asserts that the string matches the regular expression.
func (f *StringField) Matches(rx *regexp.Regexp, customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !Matches(f.val, rx) {
		msg := FormatDefaultMessage(f.v.locale, CodeInvalidMatch, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeInvalidMatch,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// In asserts that the string is one of the allowed values.
func (f *StringField) In(allowed []string, customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if !In(f.val, allowed...) {
		msg := FormatDefaultMessage(f.v.locale, CodeInvalidEnum, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeInvalidEnum,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// Custom applies a custom validation predicate to the field.
func (f *StringField) Custom(fn func(string) bool, code ErrorCode, customMsg ...string) *StringField {
	if f.skipped {
		return f
	}
	if fn != nil && !fn(f.val) {
		if code == "" {
			code = CodeCustom
		}
		msg := FormatDefaultMessage(f.v.locale, code, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    code,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// GenericField provides a fluent builder for non-string types.
type GenericField[T comparable] struct {
	v       *Validator
	field   string
	label   string
	val     T
	skipped bool
}

// FieldValue starts a fluent validation chain for any comparable type.
func FieldValue[T comparable](v *Validator, value T, field string, label ...string) *GenericField[T] {
	l := HumanizeFieldName(field)
	if len(label) > 0 && label[0] != "" {
		l = label[0]
	}
	return &GenericField[T]{
		v:     v,
		field: field,
		label: l,
		val:   value,
	}
}

// In asserts that the value is one of the allowed values.
func (f *GenericField[T]) In(allowed []T, customMsg ...string) *GenericField[T] {
	if f.skipped {
		return f
	}
	if !In(f.val, allowed...) {
		msg := FormatDefaultMessage(f.v.locale, CodeInvalidEnum, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    CodeInvalidEnum,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}

// Custom applies a custom validation predicate to the generic field.
func (f *GenericField[T]) Custom(fn func(T) bool, code ErrorCode, customMsg ...string) *GenericField[T] {
	if f.skipped {
		return f
	}
	if fn != nil && !fn(f.val) {
		if code == "" {
			code = CodeCustom
		}
		msg := FormatDefaultMessage(f.v.locale, code, f.label, nil, customMsg...)
		f.v.AddFieldError(FieldError{
			Field:   f.field,
			Label:   f.label,
			Code:    code,
			Message: msg,
		})
		f.skipped = true
	}
	return f
}
