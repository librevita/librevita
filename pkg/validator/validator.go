package validator

import (
	"context"
	"fmt"
	"strings"
)

// ErrorCode is a canonical, machine-readable validation failure code.
type ErrorCode string

const (
	CodeRequired     ErrorCode = "validation.required"
	CodeMinRunes     ErrorCode = "validation.min_runes"
	CodeMaxRunes     ErrorCode = "validation.max_runes"
	CodeBetweenRunes ErrorCode = "validation.between_runes"
	CodeInvalidEmail ErrorCode = "validation.invalid_email"
	CodeInvalidDate  ErrorCode = "validation.invalid_date"
	CodeInvalidUUID  ErrorCode = "validation.invalid_uuid"
	CodeInvalidEnum  ErrorCode = "validation.invalid_enum"
	CodeInvalidMatch ErrorCode = "validation.invalid_match"
	CodeCustom       ErrorCode = "validation.custom"
)

// FieldError describes a single field constraint violation with i18n metadata.
type FieldError struct {
	Field   string         `json:"field"`
	Label   string         `json:"label,omitempty"`
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Params  map[string]any `json:"params,omitempty"`
}

// ValidationError represents an error containing field-level and general validation failures.
type ValidationError struct {
	Msg         string                `json:"message"`
	FieldErrors map[string]FieldError `json:"field_errors,omitempty"`
	NonFieldErr string                `json:"non_field_error,omitempty"`
}

// Error formats the validation failure into a human-readable string.
func (v *ValidationError) Error() string {
	if len(v.FieldErrors) == 0 {
		if v.NonFieldErr != "" {
			return v.NonFieldErr
		}
		return v.Msg
	}
	var sb strings.Builder
	if v.NonFieldErr != "" {
		sb.WriteString(v.NonFieldErr)
	}
	for field, fe := range v.FieldErrors {
		if sb.Len() > 0 {
			sb.WriteString("; ")
		}
		fmt.Fprintf(&sb, "%s: %s", field, fe.Message)
	}
	return sb.String()
}

// FirstError returns the first available validation error message.
func (v *ValidationError) FirstError() string {
	if v.NonFieldErr != "" {
		return v.NonFieldErr
	}
	if v.Msg != "" {
		return v.Msg
	}
	for _, fe := range v.FieldErrors {
		return fe.Message
	}
	return ""
}

// Errors returns a simple map of field-to-message for backward compatibility and template rendering.
func (v *ValidationError) Errors() map[string]string {
	res := make(map[string]string, len(v.FieldErrors))
	for k, fe := range v.FieldErrors {
		res[k] = fe.Message
	}
	return res
}

// FieldError returns the structured FieldError for a field name, or nil if none exists.
func (v *ValidationError) FieldError(field string) *FieldError {
	if fe, ok := v.FieldErrors[field]; ok {
		return &fe
	}
	return nil
}

type localeContextKey struct{}

// WithLocale stores the preferred locale in the context.
func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeContextKey{}, locale)
}

// LocaleFromContext extracts the locale from the context, or returns DefaultLocale.
func LocaleFromContext(ctx context.Context) string {
	if ctx == nil {
		return DefaultLocale
	}
	if loc, ok := ctx.Value(localeContextKey{}).(string); ok && loc != "" {
		return loc
	}
	return DefaultLocale
}

// Validator collects field-specific and general validation errors.
type Validator struct {
	locale      string
	fieldErrors map[string]FieldError
	nonFieldErr string
}

// Option configures a Validator instance.
type Option func(*Validator)

// WithValidatorLocale sets the locale for message formatting.
func WithValidatorLocale(locale string) Option {
	return func(v *Validator) {
		v.locale = locale
	}
}

// New creates and initializes a new Validator instance.
func New(opts ...Option) *Validator {
	v := &Validator{
		locale:      DefaultLocale,
		fieldErrors: make(map[string]FieldError),
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// FromContext creates a Validator using the locale present in ctx.
func FromContext(ctx context.Context, opts ...Option) *Validator {
	loc := LocaleFromContext(ctx)
	v := New(append([]Option{WithValidatorLocale(loc)}, opts...)...)
	return v
}

// Valid reports whether no errors have been recorded.
func (v *Validator) Valid() bool {
	return len(v.fieldErrors) == 0 && v.nonFieldErr == ""
}

// HasErrors reports whether any validation errors exist.
func (v *Validator) HasErrors() bool {
	return !v.Valid()
}

// AddFieldError records a FieldError for a field if one doesn't already exist (first error wins).
func (v *Validator) AddFieldError(fe FieldError) {
	if fe.Field == "" {
		v.AddNonFieldError(fe.Message)
		return
	}
	if _, exists := v.fieldErrors[fe.Field]; !exists {
		v.fieldErrors[fe.Field] = fe
	}
}

// AddError records a field error with custom message and CodeCustom.
func (v *Validator) AddError(field, message string) {
	v.AddFieldError(FieldError{
		Field:   field,
		Label:   HumanizeFieldName(field),
		Code:    CodeCustom,
		Message: message,
	})
}

// AddNonFieldError records a general, non-field-specific form error.
func (v *Validator) AddNonFieldError(message string) {
	if v.nonFieldErr == "" {
		v.nonFieldErr = message
	}
}

// Check evaluates a condition and records a field error if the condition is false.
func (v *Validator) Check(ok bool, field, message string) {
	if !ok {
		v.AddError(field, message)
	}
}

// Validatable defines a domain entity or enum that can validate itself.
type Validatable interface {
	Valid() bool
}

// Validatable asserts that val is non-nil and its Valid() method returns true.
func (v *Validator) Validatable(val Validatable, field string, labelAndMsg ...string) {
	if val != nil && val.Valid() {
		return
	}
	label := HumanizeFieldName(field)
	var customMsg string
	if len(labelAndMsg) > 0 && labelAndMsg[0] != "" {
		label = labelAndMsg[0]
	}
	if len(labelAndMsg) > 1 {
		customMsg = labelAndMsg[1]
	}
	msg := FormatDefaultMessage(v.locale, CodeInvalidEnum, label, nil, customMsg)
	v.AddFieldError(FieldError{
		Field:   field,
		Label:   label,
		Code:    CodeInvalidEnum,
		Message: msg,
	})
}

// CheckNonField evaluates a condition and records a non-field error if the condition is false.
func (v *Validator) CheckNonField(ok bool, message string) {
	if !ok {
		v.AddNonFieldError(message)
	}
}

// FieldError returns the error message for the specified field, or an empty string if none exists.
func (v *Validator) FieldError(field string) string {
	if fe, ok := v.fieldErrors[field]; ok {
		return fe.Message
	}
	return ""
}

// FirstError returns the first recorded error message (general or field-specific).
func (v *Validator) FirstError() string {
	if v.nonFieldErr != "" {
		return v.nonFieldErr
	}
	for _, fe := range v.fieldErrors {
		return fe.Message
	}
	return ""
}

// Err returns a *ValidationError if any errors occurred, or nil if validation succeeded.
func (v *Validator) Err() error {
	if v.Valid() {
		return nil
	}
	return &ValidationError{
		Msg:         v.FirstError(),
		FieldErrors: v.fieldErrors,
		NonFieldErr: v.nonFieldErr,
	}
}
