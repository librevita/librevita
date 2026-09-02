package ident_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func identDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}

func runGen(t *testing.T, args ...string) (string, error) {
	t.Helper()
	dir := identDir(t)
	cmdArgs := append([]string{"run", "-mod=mod", filepath.Join(dir, "gen.go")}, args...)
	// #nosec G204 -- argv is "go run gen.go" plus test-controlled flags.
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	// #nosec G304 -- path is codec_gen.go or t.TempDir.
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func TestGenerateTypes(t *testing.T) {
	dir := identDir(t)
	out := filepath.Join(t.TempDir(), "codec_gen.go")
	stderr, err := runGen(t, "-types", filepath.Join(dir, "types.go"), "-out", out)
	require.NoError(t, err, stderr)
	got := string(readFile(t, out))
	require.Contains(t, got, "func ParseClinic(")
	require.Contains(t, got, "func MustParseStaffChangeRequest(")
}

func TestGenerateRejects(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name:    "not uuid",
			src:     "package ident\n\ntype FooID string\n",
			wantErr: "FooID: must be defined as uuid.UUID",
		},
		{
			name:    "missing ID suffix",
			src:     "package ident\n\nimport \"github.com/google/uuid\"\n\ntype Foo uuid.UUID\n",
			wantErr: "Foo: type name must end with ID",
		},
		{
			name:    "empty stem",
			src:     "package ident\n\nimport \"github.com/google/uuid\"\n\ntype ID uuid.UUID\n",
			wantErr: "ID: type name must end with ID",
		},
		{
			name:    "alias",
			src:     "package ident\n\nimport \"github.com/google/uuid\"\n\ntype FooID = uuid.UUID\n",
			wantErr: "FooID: must be a defined type, not an alias",
		},
		{
			name:    "empty",
			src:     "package ident\n",
			wantErr: "no uuid.UUID defined types",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			typesPath := filepath.Join(tmp, "types.go")
			require.NoError(t, os.WriteFile(typesPath, []byte(tc.src), 0o600))

			stderr, err := runGen(t, "-types", typesPath, "-out", filepath.Join(tmp, "codec_gen.go"))
			require.Error(t, err)
			require.Contains(t, stderr, tc.wantErr)
		})
	}
}
