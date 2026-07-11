// Package schema holds the per-table field layouts (ported from the server
// source) and a name->schema registry used by the `table` command.
package schema

import "github.com/idevelopthings/bdo-data-extractor/internal/bss"

// Registry maps a lowercased table basename (no extension) to its schema.
var Registry = map[string]*bss.Schema{}

func register(s *bss.Schema) { Registry[s.Name] = s }
