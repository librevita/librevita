package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	atlasmigrate "ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/jackc/pgx/v5/stdlib"
	"modernc.org/sqlite"

	"librevita.org/ent/migrate"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
	sql.Register("postgres", stdlib.GetDefaultDriver())
}

type prettyGooseFormatter struct{}

func (f *prettyGooseFormatter) Format(plan *atlasmigrate.Plan) ([]atlasmigrate.File, error) {
	var buf strings.Builder
	buf.WriteString("-- +goose Up\n")
	for _, c := range plan.Changes {
		if c.Comment != "" {
			buf.WriteString("-- " + c.Comment + "\n")
		}
		formatted := formatSQL(c.Cmd)
		buf.WriteString(formatted + ";\n\n")
	}

	if plan.Reversible {
		buf.WriteString("-- +goose Down\n")
		for i := len(plan.Changes) - 1; i >= 0; i-- {
			c := plan.Changes[i]
			revs, err := c.ReverseStmts()
			if err != nil {
				return nil, err
			}
			if len(revs) > 0 && c.Comment != "" {
				buf.WriteString("-- reverse: " + c.Comment + "\n")
			}
			for _, r := range revs {
				formatted := formatSQL(r)
				buf.WriteString(formatted + ";\n")
			}
		}
	}

	version := plan.Version
	if version == "" {
		version = time.Now().UTC().Format("20060102150405")
	}
	filename := fmt.Sprintf("%s_%s.sql", version, plan.Name)
	content := []byte(buf.String())
	return []atlasmigrate.File{
		&gooseFile{
			name:    filename,
			version: version,
			desc:    plan.Name,
			content: content,
		},
	}, nil
}

type gooseFile struct {
	name    string
	version string
	desc    string
	content []byte
}

func (f *gooseFile) Name() string    { return f.name }
func (f *gooseFile) Version() string { return f.version }
func (f *gooseFile) Desc() string    { return f.desc }
func (f *gooseFile) Bytes() []byte   { return f.content }
func (f *gooseFile) Stmts() ([]string, error) {
	return (&sqltool.GooseFile{LocalFile: atlasmigrate.NewLocalFile(f.name, f.content)}).Stmts()
}
func (f *gooseFile) StmtDecls() ([]*atlasmigrate.Stmt, error) {
	return (&sqltool.GooseFile{LocalFile: atlasmigrate.NewLocalFile(f.name, f.content)}).StmtDecls()
}

func formatSQL(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	stmt = strings.TrimSuffix(stmt, ";")
	upper := strings.ToUpper(stmt)
	if formatted, ok := formatCreateTable(stmt, upper); ok {
		return formatted
	}
	if formatted, ok := formatAlterTable(stmt, upper); ok {
		return formatted
	}
	return stmt
}

func formatCreateTable(stmt, upper string) (string, bool) {
	if !strings.HasPrefix(upper, "CREATE TABLE") {
		return "", false
	}
	openIdx := strings.Index(stmt, "(")
	closeIdx := strings.LastIndex(stmt, ")")
	if openIdx == -1 || closeIdx <= openIdx {
		return "", false
	}
	prefix := strings.TrimSpace(stmt[:openIdx])
	body := stmt[openIdx+1 : closeIdx]
	suffix := strings.TrimSpace(stmt[closeIdx+1:])
	res := prefix + " (\n" + joinSQLElements(body) + "\n)"
	if suffix != "" {
		res += " " + suffix
	}
	return res, true
}

func formatAlterTable(stmt, upper string) (string, bool) {
	if !strings.HasPrefix(upper, "ALTER TABLE") {
		return "", false
	}
	parts := strings.Fields(stmt)
	if len(parts) < 3 {
		return "", false
	}
	tableIdx := 2
	if strings.ToUpper(parts[2]) == "ONLY" && len(parts) >= 4 {
		tableIdx = 3
	}
	tableEndIdx := strings.Index(stmt, parts[tableIdx]) + len(parts[tableIdx])
	prefix := strings.TrimSpace(stmt[:tableEndIdx])
	body := strings.TrimSpace(stmt[tableEndIdx:])
	if len(splitTopLevelCommas(body)) <= 1 {
		return "", false
	}
	return prefix + "\n" + joinSQLElements(body), true
}

func joinSQLElements(body string) string {
	elements := splitTopLevelCommas(body)
	var formattedBody []string
	for _, el := range elements {
		trimmed := strings.TrimSpace(el)
		if trimmed != "" {
			formattedBody = append(formattedBody, "  "+trimmed)
		}
	}
	return strings.Join(formattedBody, ",\n")
}

type commaSplitter struct {
	parts         []string
	current       strings.Builder
	depth         int
	inSingleQuote bool
	inDoubleQuote bool
	inBacktick    bool
}

func (s *commaSplitter) inQuote() bool {
	return s.inSingleQuote || s.inDoubleQuote || s.inBacktick
}

func (s *commaSplitter) feed(ch byte) {
	switch ch {
	case '\'':
		s.toggleSingle()
	case '"':
		s.toggleDouble()
	case '`':
		s.toggleBacktick()
	case '(':
		s.openParen()
	case ')':
		s.closeParen()
	case ',':
		s.comma()
	default:
		s.current.WriteByte(ch)
	}
}

func (s *commaSplitter) toggleSingle() {
	if !s.inDoubleQuote && !s.inBacktick {
		s.inSingleQuote = !s.inSingleQuote
	}
	s.current.WriteByte('\'')
}

func (s *commaSplitter) toggleDouble() {
	if !s.inSingleQuote && !s.inBacktick {
		s.inDoubleQuote = !s.inDoubleQuote
	}
	s.current.WriteByte('"')
}

func (s *commaSplitter) toggleBacktick() {
	if !s.inSingleQuote && !s.inDoubleQuote {
		s.inBacktick = !s.inBacktick
	}
	s.current.WriteByte('`')
}

func (s *commaSplitter) openParen() {
	if !s.inQuote() {
		s.depth++
	}
	s.current.WriteByte('(')
}

func (s *commaSplitter) closeParen() {
	if !s.inQuote() {
		s.depth--
	}
	s.current.WriteByte(')')
}

func (s *commaSplitter) comma() {
	if s.depth == 0 && !s.inQuote() {
		s.parts = append(s.parts, s.current.String())
		s.current.Reset()
		return
	}
	s.current.WriteByte(',')
}

func splitTopLevelCommas(s string) []string {
	var sp commaSplitter
	for i := 0; i < len(s); i++ {
		sp.feed(s[i])
	}
	if sp.current.Len() > 0 {
		sp.parts = append(sp.parts, sp.current.String())
	}
	return sp.parts
}

func main() {
	name, dir, devURL, dial, rehash := parseMigrateFlags()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Fatalf("failed creating migrations directory %q: %v", dir, err)
	}

	gooseDir, err := sqltool.NewGooseDir(dir)
	if err != nil {
		log.Fatalf("failed creating goose migrations dir: %v", err)
	}

	if rehash {
		runRehash(gooseDir, dir)
		return
	}

	name = requireMigrationName(name)
	d := dialect.SQLite
	if dial == "postgres" {
		d = dialect.Postgres
	}
	devURL = resolveDevURL(devURL, d)
	resetPostgresPublic(d, devURL)
	generateMigration(devURL, name, dir, gooseDir, d)
}

func parseMigrateFlags() (name, dir, devURL, dial string, rehash bool) {
	flag.StringVar(&name, "name", "", "migration name (required unless --rehash is used)")
	flag.StringVar(&dir, "dir", "internal/database/migrations/sqlite", "migrations output directory")
	flag.StringVar(&devURL, "dev-url", "", "dev database URL (default: in-memory sqlite)")
	flag.StringVar(&dial, "dialect", "sqlite", "SQL dialect: sqlite or postgres")
	flag.BoolVar(&rehash, "rehash", false, "recalculate and write atlas.sum for the migrations directory")
	flag.Parse()
	return name, dir, devURL, dial, rehash
}

func runRehash(gooseDir atlasmigrate.Dir, dir string) {
	sum, err := gooseDir.Checksum()
	if err != nil {
		log.Fatalf("failed calculating checksum: %v", err)
	}
	if err := atlasmigrate.WriteSumFile(gooseDir, sum); err != nil {
		log.Fatalf("failed writing atlas.sum: %v", err)
	}
	fmt.Printf("Successfully updated atlas.sum in %s\n", dir)
}

func requireMigrationName(name string) string {
	if name == "" && flag.NArg() > 0 {
		name = flag.Arg(0)
	}
	if name != "" {
		return name
	}
	fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/migrate [flags] <migration_name>\n\nFlags:\n")
	flag.PrintDefaults()
	os.Exit(1)
	return name
}

func resolveDevURL(devURL, d string) string {
	if devURL != "" {
		return devURL
	}
	if d == dialect.Postgres {
		return postgresDevURL()
	}
	return "sqlite://file?mode=memory&cache=shared&_pragma=foreign_keys(1)"
}

func postgresDevURL() string {
	if u := os.Getenv("ATLAS_DEV_URL"); u != "" {
		return u
	}
	if u := os.Getenv("POSTGRES_DEV_URL"); u != "" {
		return u
	}
	// Default to standard local postgres ports
	// #nosec G101 -- default local dev postgres connection string used by the migration CLI in local development, not a production secret.
	return "postgres://postgres:postgres@localhost:5433/dev?sslmode=disable"
}

func resetPostgresPublic(d, devURL string) {
	if d != dialect.Postgres {
		return
	}
	devDB, err := sql.Open("postgres", devURL)
	if err != nil {
		return
	}
	_, _ = devDB.ExecContext(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	_ = devDB.Close()
}

func generateMigration(devURL, name, dir string, gooseDir atlasmigrate.Dir, d string) {
	opts := []schema.MigrateOption{
		schema.WithDir(gooseDir),
		schema.WithFormatter(&prettyGooseFormatter{}),
		schema.WithMigrationMode(schema.ModeReplay),
		schema.WithDialect(d),
		schema.WithDropColumn(true),
		schema.WithDropIndex(true),
	}
	if err := migrate.NamedDiff(context.Background(), devURL, name, opts...); err != nil {
		log.Fatalf("failed generating migration diff: %v", err)
	}
	fmt.Printf("Successfully generated migration %q in %s\n", name, dir)
}
