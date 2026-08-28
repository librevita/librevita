package validator

import (
	"fmt"
	"strings"
)

// Supported default locales.
const (
	LocaleEN   = "en"
	LocalePTBR = "pt-BR"
)

// DefaultLocale is used when no locale is explicitly specified.
const DefaultLocale = LocaleEN

// Catalog holds message templates per error code for a locale.
type Catalog map[ErrorCode]string

var defaultCatalogs = map[string]Catalog{
	LocaleEN: {
		CodeRequired:     "{label} is required",
		CodeMinRunes:     "{label} must be at least {min} characters",
		CodeMaxRunes:     "{label} must be at most {max} characters",
		CodeBetweenRunes: "{label} must be between {min} and {max} characters",
		CodeInvalidEmail: "enter a valid email address",
		CodeInvalidDate:  "enter a valid date (YYYY-MM-DD)",
		CodeInvalidUUID:  "enter a valid UUID",
		CodeInvalidEnum:  "invalid {label}",
		CodeInvalidMatch: "{label} has an invalid format",
		CodeCustom:       "{label} is invalid",
	},
	LocalePTBR: {
		CodeRequired:     "{label} é obrigatório",
		CodeMinRunes:     "{label} deve ter no mínimo {min} caracteres",
		CodeMaxRunes:     "{label} deve ter no máximo {max} caracteres",
		CodeBetweenRunes: "{label} deve ter entre {min} e {max} caracteres",
		CodeInvalidEmail: "informe um endereço de e-mail válido",
		CodeInvalidDate:  "informe uma data válida (AAAA-MM-DD)",
		CodeInvalidUUID:  "informe um UUID válido",
		CodeInvalidEnum:  "{label} inválido",
		CodeInvalidMatch: "{label} possui formato inválido",
		CodeCustom:       "{label} é inválido",
	},
}

// NormalizeLocale standardizes locale strings (e.g. "pt_BR", "pt-br", "pt" -> "pt-BR").
func NormalizeLocale(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultLocale
	}
	low := strings.ToLower(strings.ReplaceAll(raw, "_", "-"))
	if strings.HasPrefix(low, "pt") {
		return LocalePTBR
	}
	return LocaleEN
}

// FormatDefaultMessage formats an error message using the locale catalog and parameters.
func FormatDefaultMessage(locale string, code ErrorCode, label string, params map[string]any, customMsg ...string) string {
	if len(customMsg) > 0 && strings.TrimSpace(customMsg[0]) != "" {
		return strings.TrimSpace(customMsg[0])
	}

	norm := NormalizeLocale(locale)
	cat, ok := defaultCatalogs[norm]
	if !ok {
		cat = defaultCatalogs[DefaultLocale]
	}

	tmpl, ok := cat[code]
	if !ok {
		tmpl = defaultCatalogs[DefaultLocale][code]
		if tmpl == "" {
			return fmt.Sprintf("%s is invalid", label)
		}
	}

	msg := strings.ReplaceAll(tmpl, "{label}", label)
	for k, v := range params {
		msg = strings.ReplaceAll(msg, "{"+k+"}", fmt.Sprint(v))
	}
	return msg
}

// HumanizeFieldName converts snake_case or kebab-case field names to readable labels.
func HumanizeFieldName(field string) string {
	s := strings.ReplaceAll(field, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	return strings.TrimSpace(s)
}
