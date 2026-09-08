package client

import (
	_ "embed"

	"entgo.io/ent/entc/gen"
)

//go:embed client.tmpl
var clientTemplate string

// Template overrides Ent's default client template to remove the migrate.Schema field
// from the Client struct, preventing ariga.io/atlas from being compiled into the web binary.
var Template = gen.MustParse(gen.NewTemplate("client_override").Parse(clientTemplate))
