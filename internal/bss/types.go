// Package bss parses BDO's .bss/.dbss table format: a PABR container with a
// string table at the end and records made of ordered, typed fields.
package bss

// ValueType is a field's wire type
type ValueType int

const (
	Text    ValueType = iota // int32 index into string table (UTF-8)
	UtfText                  // int32 index into string table (UTF-16)
	UniText                  // int32 index into string table (UTF-32)
	Byte
	Int16
	UInt16
	Int32
	UInt32
	Int64
	Float
	Bytes // fixed-size blob (uses Field.Size)
)

// Field is one column in a table schema.
type Field struct {
	Name string
	Type ValueType
	Size int // only for Bytes
}

// Schema is an ordered list of fields for one table.
type Schema struct {
	Name   string
	Fields []Field
}

// builder helpers mirror Tables.addField(...) for concise schema definitions.

// Add appends a named field.
func (s *Schema) Add(t ValueType, name string) *Schema {
	s.Fields = append(s.Fields, Field{Name: name, Type: t})
	return s
}

// Anon appends an unnamed field (auto-named Unk<index> at read time).
func (s *Schema) Anon(t ValueType) *Schema { return s.Add(t, "Unk") }

// AddBytes appends a fixed-size blob field.
func (s *Schema) AddBytes(size int) *Schema {
	s.Fields = append(s.Fields, Field{Name: "Unk", Type: Bytes, Size: size})
	return s
}

// AddBytesNamed appends a fixed-size blob field retrievable by name.
func (s *Schema) AddBytesNamed(name string, size int) *Schema {
	s.Fields = append(s.Fields, Field{Name: name, Type: Bytes, Size: size})
	return s
}

// New starts a schema.
func New(name string) *Schema { return &Schema{Name: name} }
