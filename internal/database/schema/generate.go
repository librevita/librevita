package schema

import (
	"entgo.io/ent/dialect"
)

//go:generate go run -mod=mod entc.go

var blobType = map[string]string{
	dialect.SQLite:   "blob",
	dialect.Postgres: "bytea",
}
