package normalize_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"librevita.org/internal/core/normalize"
)

func TestPhone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"+55 (11) 98765-4321", "5511987654321"},
		{"(21) 91234-5678", "21912345678"},
		{"  +1 (555) 234-5678  ", "15552345678"},
		{"1234567890", "1234567890"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Phone(tt.input))
	}
}

func TestEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"  Doctor.John+clinic@EXAMPLE.COM  ", "doctor.john+clinic@example.com"},
		{"patient.name@hospital.org.br", "patient.name@hospital.org.br"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Email(tt.input))
	}
}

func TestDocument(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"123.456.789-00", "12345678900"},
		{"12.345.678/0001-90", "12345678000190"},
		{"  AB-123.456  ", "ab123456"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Document(tt.input))
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"  Maria Joana da Silva  ", "maria joana da silva"},
		{"Dr. John Doe", "dr. john doe"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Text(tt.input))
	}
}
