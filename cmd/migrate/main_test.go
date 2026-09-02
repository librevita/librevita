package main

import (
	"testing"

	atlasmigrate "ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatSQL(t *testing.T) {
	// 1. CREATE TABLE
	createSQL := "CREATE TABLE `users` (`id` uuid NOT NULL, `email` varchar(255) NOT NULL, PRIMARY KEY (`id`));"
	formatted := formatSQL(createSQL)
	assert.Contains(t, formatted, "CREATE TABLE `users` (\n")
	assert.Contains(t, formatted, "  `id` uuid NOT NULL,\n")

	// 2. ALTER TABLE
	alterSQL := "ALTER TABLE `users` ADD COLUMN `age` int, ADD COLUMN `status` varchar(50);"
	formattedAlter := formatSQL(alterSQL)
	assert.Contains(t, formattedAlter, "ALTER TABLE `users`\n")

	// 3. Simple statements
	simpleSQL := "DROP TABLE `old_table`;"
	assert.Equal(t, "DROP TABLE `old_table`", formatSQL(simpleSQL))
}

func TestSplitTopLevelCommas(t *testing.T) {
	input := "`id` uuid NOT NULL, `payload` jsonb DEFAULT '{\"a\": 1, \"b\": 2}', `count` int"
	parts := splitTopLevelCommas(input)
	assert.Len(t, parts, 3)
	assert.Equal(t, "`id` uuid NOT NULL", parts[0])
	assert.Equal(t, " `payload` jsonb DEFAULT '{\"a\": 1, \"b\": 2}'", parts[1])
	assert.Equal(t, " `count` int", parts[2])
}

func TestPrettyGooseFormatter(t *testing.T) {
	formatter := &prettyGooseFormatter{}
	plan := &atlasmigrate.Plan{
		Name:    "add_users_table",
		Version: "20260902120000",
		Changes: []*atlasmigrate.Change{
			{
				Cmd:     "CREATE TABLE `users` (`id` uuid NOT NULL);",
				Comment: "create users table",
			},
		},
	}

	files, err := formatter.Format(plan)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	assert.Equal(t, "20260902120000_add_users_table.sql", f.Name())
	assert.Equal(t, "20260902120000", f.Version())
	assert.Equal(t, "add_users_table", f.Desc())
	assert.Contains(t, string(f.Bytes()), "-- +goose Up")
	assert.Contains(t, string(f.Bytes()), "-- create users table")
}

func TestGooseFileMethods(t *testing.T) {
	gf := &gooseFile{
		name:    "test.sql",
		version: "1",
		desc:    "test",
		content: []byte("-- +goose Up\nSELECT 1;\n"),
	}
	stmts, err := gf.Stmts()
	require.NoError(t, err)
	assert.NotEmpty(t, stmts)

	decls, err := gf.StmtDecls()
	require.NoError(t, err)
	assert.NotEmpty(t, decls)
}

func TestResolveDevURL(t *testing.T) {
	assert.Equal(t, "custom-url", resolveDevURL("custom-url", dialect.SQLite))
	assert.Contains(t, resolveDevURL("", dialect.SQLite), "sqlite://file")
	assert.Contains(t, resolveDevURL("", dialect.Postgres), "postgres://")
}

func TestMigrateCLIHelpers(t *testing.T) {
	// 1. requireMigrationName
	assert.Equal(t, "add_test_table", requireMigrationName("add_test_table"))

	// 2. postgresDevURL with env
	t.Setenv("ATLAS_DEV_URL", "postgres://custom:5432/atlas")
	assert.Equal(t, "postgres://custom:5432/atlas", postgresDevURL())

	t.Setenv("ATLAS_DEV_URL", "")
	t.Setenv("POSTGRES_DEV_URL", "postgres://custom:5432/pg")
	assert.Equal(t, "postgres://custom:5432/pg", postgresDevURL())

	t.Setenv("POSTGRES_DEV_URL", "")
	assert.Contains(t, postgresDevURL(), "postgres://postgres:postgres@localhost:5433/dev")

	// 3. resetPostgresPublic noop on SQLite
	resetPostgresPublic(dialect.SQLite, "sqlite://file")

	// 4. runRehash
	tmpDir := t.TempDir()
	gooseDir, err := sqltool.NewGooseDir(tmpDir)
	require.NoError(t, err)
	runRehash(gooseDir, tmpDir)
}

func TestFormatSQLEdgeCases(t *testing.T) {
	// 1. ALTER TABLE ONLY
	alterOnly := "ALTER TABLE ONLY `users` ADD COLUMN `a` int, ADD COLUMN `b` text;"
	res := formatSQL(alterOnly)
	assert.Contains(t, res, "ALTER TABLE ONLY `users`\n")

	// 2. ALTER TABLE single element (not multi-line)
	alterSingle := "ALTER TABLE `users` ADD COLUMN `c` int;"
	assert.Equal(t, "ALTER TABLE `users` ADD COLUMN `c` int", formatSQL(alterSingle))

	// 3. CREATE TABLE with suffix
	createWithSuffix := "CREATE TABLE `users` (`id` int) ENGINE=InnoDB;"
	res = formatSQL(createWithSuffix)
	assert.Contains(t, res, "ENGINE=InnoDB")

	// 4. Malformed CREATE TABLE (no matching parens)
	malformed := "CREATE TABLE `broken`"
	assert.Equal(t, "CREATE TABLE `broken`", formatSQL(malformed))

	// 5. Short ALTER TABLE
	shortAlter := "ALTER TABLE"
	assert.Equal(t, "ALTER TABLE", formatSQL(shortAlter))
}

func TestPrettyGooseFormatterReversible(t *testing.T) {
	formatter := &prettyGooseFormatter{}
	plan := &atlasmigrate.Plan{
		Name:       "reversible_migration",
		Reversible: true,
		Changes: []*atlasmigrate.Change{
			{
				Cmd:     "CREATE TABLE `rev` (`id` int);",
				Comment: "create rev",
				Reverse: "DROP TABLE `rev`;",
			},
		},
	}
	files, err := formatter.Format(plan)
	require.NoError(t, err)
	require.Len(t, files, 1)
	content := string(files[0].Bytes())
	assert.Contains(t, content, "-- +goose Up")
	assert.Contains(t, content, "-- +goose Down")
	assert.Contains(t, content, "DROP TABLE `rev`")
}
