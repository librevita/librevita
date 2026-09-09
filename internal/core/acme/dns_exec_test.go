package acme

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecProvider_PresentAndCleanUp(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "exec.log")
	scriptFile := filepath.Join(dir, "hook.sh")

	script := `#!/bin/sh
echo "$1 $2 $3 $LIBREVITA_ACME_RECORD_NAME" >> ` + logFile + `
exit 0
`
	require.NoError(t, os.WriteFile(scriptFile, []byte(script), 0o700)) // #nosec G306 -- Test script requires execute permission.

	p := NewExecProvider(scriptFile)

	err := p.Present(ctx, "example.org", "val123")
	require.NoError(t, err)

	err = p.CleanUp(ctx, "example.org", "val123")
	require.NoError(t, err)

	data, err := os.ReadFile(logFile) // #nosec G304 -- Test log file path within t.TempDir.
	require.NoError(t, err)
	expected := "present example.org val123 _acme-challenge.example.org\ncleanup example.org val123 _acme-challenge.example.org\n"
	assert.Equal(t, expected, string(data))

	// Error path
	failingScript := filepath.Join(dir, "fail.sh")
	require.NoError(t, os.WriteFile(failingScript, []byte("#!/bin/sh\nexit 1\n"), 0o700)) // #nosec G306 -- Test script requires execute permission.
	failProvider := NewExecProvider(failingScript)
	assert.Error(t, failProvider.Present(ctx, "example.org", "val123"))
}
