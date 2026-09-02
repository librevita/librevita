package main

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"librevita.org/internal/core/config"
)

func TestWebMainFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.RegisterFlags(fs)
	assert.NotNil(t, fs.Lookup("config"))
}
