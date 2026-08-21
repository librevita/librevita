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
	if strings.HasPrefix(upper, "CREATE TABLE") {
		openIdx := strings.Index(stmt, "(")
		closeIdx := strings.LastIndex(stmt, ")")
		if openIdx != -1 && closeIdx > openIdx {
			prefix := strings.TrimSpace(stmt[:openIdx])
			body := stmt[openIdx+1 : closeIdx]
			suffix := strings.TrimSpace(stmt[closeIdx+1:])

			elements := splitTopLevelCommas(body)
			var formattedBody []string
			for _, el := range elements {
				trimmed := strings.TrimSpace(el)
				if trimmed != "" {
					formattedBody = append(formattedBody, "  "+trimmed)
				}
			}
			res := prefix + " (\n" + strings.Join(formattedBody, ",\n") + "\n)"
			if suffix != "" {
				res += " " + suffix
			}
			return res
		}
	}

	if strings.HasPrefix(upper, "ALTER TABLE") {
		parts := strings.Fields(stmt)
		if len(parts) >= 3 {
			tableIdx := 2
			if strings.ToUpper(parts[2]) == "ONLY" && len(parts) >= 4 {
				tableIdx = 3
			}
			tableEndIdx := strings.Index(stmt, parts[tableIdx]) + len(parts[tableIdx])
			prefix := strings.TrimSpace(stmt[:tableEndIdx])
			body := strings.TrimSpace(stmt[tableEndIdx:])

			elements := splitTopLevelCommas(body)
			if len(elements) > 1 {
				var formattedBody []string
				for _, el := range elements {
					trimmed := strings.TrimSpace(el)
					if trimmed != "" {
						formattedBody = append(formattedBody, "  "+trimmed)
					}
				}
				return prefix + "\n" + strings.Join(formattedBody, ",\n")
			}
		}
	}

	return stmt
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\'':
			if !inDoubleQuote && !inBacktick {
				inSingleQuote = !inSingleQuote
			}
			current.WriteByte(ch)
		case '"':
			if !inSingleQuote && !inBacktick {
				inDoubleQuote = !inDoubleQuote
			}
			current.WriteByte(ch)
		case '`':
			if !inSingleQuote && !inDoubleQuote {
				inBacktick = !inBacktick
			}
			current.WriteByte(ch)
		case '(':
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				depth++
			}
			current.WriteByte(ch)
		case ')':
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				depth--
			}
			current.WriteByte(ch)
		case ',':
			if depth == 0 && !inSingleQuote && !inDoubleQuote && !inBacktick {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func main() {
	var (
		name   string
		dir    string
		devURL string
		dial   string
		rehash bool
	)
	flag.StringVar(&name, "name", "", "migration name (required unless --rehash is used)")
	flag.StringVar(&dir, "dir", "internal/database/migrations/sqlite", "migrations output directory")
	flag.StringVar(&devURL, "dev-url", "", "dev database URL (default: in-memory sqlite)")
	flag.StringVar(&dial, "dialect", "sqlite", "SQL dialect: sqlite or postgres")
	flag.BoolVar(&rehash, "rehash", false, "recalculate and write atlas.sum for the migrations directory")
	flag.Parse()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Fatalf("failed creating migrations directory %q: %v", dir, err)
	}

	gooseDir, err := sqltool.NewGooseDir(dir)
	if err != nil {
		log.Fatalf("failed creating goose migrations dir: %v", err)
	}

	if rehash {
		sum, err := gooseDir.Checksum()
		if err != nil {
			log.Fatalf("failed calculating checksum: %v", err)
		}
		if err := atlasmigrate.WriteSumFile(gooseDir, sum); err != nil {
			log.Fatalf("failed writing atlas.sum: %v", err)
		}
		fmt.Printf("Successfully updated atlas.sum in %s\n", dir)
		return
	}

	if name == "" && flag.NArg() > 0 {
		name = flag.Arg(0)
	}
	if name == "" {
		fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/migrate [flags] <migration_name>\n\nFlags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	d := dialect.SQLite
	if dial == "postgres" {
		d = dialect.Postgres
	}

	if devURL == "" {
		if d == dialect.Postgres {
			devURL = os.Getenv("ATLAS_DEV_URL")
			if devURL == "" {
				devURL = os.Getenv("POSTGRES_DEV_URL")
			}
			if devURL == "" {
				// Default to standard local postgres ports
				// #nosec G101 -- default local dev postgres connection string used by the migration CLI in local development, not a production secret.
				devURL = "postgres://postgres:postgres@localhost:5433/dev?sslmode=disable"
			}
		} else {
			devURL = "sqlite://file?mode=memory&cache=shared&_pragma=foreign_keys(1)"
		}
	}

	if d == dialect.Postgres {
		if devDB, err := sql.Open("postgres", devURL); err == nil {
			_, _ = devDB.ExecContext(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
			_ = devDB.Close()
		}
	}

	opts := []schema.MigrateOption{
		schema.WithDir(gooseDir),
		schema.WithFormatter(&prettyGooseFormatter{}),
		schema.WithMigrationMode(schema.ModeReplay),
		schema.WithDialect(d),
		schema.WithDropColumn(true),
		schema.WithDropIndex(true),
	}

	ctx := context.Background()
	if err := migrate.NamedDiff(ctx, devURL, name, opts...); err != nil {
		log.Fatalf("failed generating migration diff: %v", err)
	}
	fmt.Printf("Successfully generated migration %q in %s\n", name, dir)
}
