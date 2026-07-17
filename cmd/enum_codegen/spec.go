package main

import (
	"fmt"
	"go/types"
	"log"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// spec is one parsed enum definition file.
type spec struct {
	// Package is the Go package the generated file belongs to.
	Package string `yaml:"package"`
	// Type is the generated enum type name, e.g. SlotName.
	Type string `yaml:"type"`
	// Underlying is the enum's underlying type; defaults to int. Any
	// predeclared integer, float, or string type is allowed. For string
	// underlyings, wire values are emitted as quoted string literals.
	Underlying string `yaml:"underlying"`
	// MarshalText adds encoding.TextMarshaler/Unmarshaler methods when set.
	MarshalText bool `yaml:"marshalText"`
	// Case controls how a member name becomes its exported Go/TS identifier:
	// "pascal" (default, suits camelCase names), "snake"/"preserve" (verbatim —
	// keeps UPPER_SNAKE names as authored, avoiding pascal collisions), "camel",
	// or "lower". The enum type and yaml field names are always pascal.
	Case string `yaml:"case"`
	// Container overrides the container var name; defaults to Type + "s".
	Container *string `yaml:"container"`
	// Fields declares explicit metadata-column types, keyed by yaml key. A
	// declared type overrides inference; columns left out stay inferred. A type
	// is a predeclared basic type, another generated enum's type name (a
	// reference), or a slice of either (e.g. StatId, []StatId, int64, []int).
	Fields map[string]string `yaml:"fields"`
	// Iterators names metadata columns to expose as extra iterators over their
	// values (Go iter.Seq method, TS generator function), in declaration order.
	Iterators []string `yaml:"iterators"`
	// Out is the Go output directory, relative to this file; defaults to
	// writing alongside the yaml. Overridden by the -out / -root flags.
	Out string `yaml:"out"`
	// TSOut, when set, is the TypeScript output file, relative to this file.
	TSOut string `yaml:"tsOut"`
	// TSModule overrides the module specifier other enums use to import this
	// enum's TypeScript symbols. Defaults to the relative path of TSOut; set it
	// when the symbols live elsewhere (e.g. hand-written, or behind a path alias).
	TSModule string `yaml:"tsModule"`
	// Values are the enum members, in declaration order.
	Values []yaml.Node `yaml:"values"`
}

// underlyingInfo classifies the declared underlying type via the predeclared
// universe, so every integer width (byte, rune, int8..int64, uint8..uintptr),
// the float types, and string are handled uniformly rather than by name. It
// returns 0 for a type that is not a predeclared basic type.
func (s *spec) underlyingInfo() types.BasicInfo {
	return basicInfo(s.Underlying)
}

// stringUnderlying reports whether wire values are string literals.
func (s *spec) stringUnderlying() bool {
	return s.underlyingInfo()&types.IsString != 0
}

// fmtVerb is the fmt verb used to print a raw underlying value in String() and
// the error paths: %q for strings, %g for floats, %d for integers.
func (s *spec) fmtVerb() string {
	switch info := s.underlyingInfo(); {
	case info&types.IsString != 0:
		return "%q"
	case info&types.IsFloat != 0:
		return "%g"
	default:
		return "%d"
	}
}

// basicInfo returns the predeclared-universe basic-kind flags for name, or 0
// when name is not a predeclared basic type.
func basicInfo(name string) types.BasicInfo {
	obj := types.Universe.Lookup(name)
	if obj == nil {
		return 0
	}
	if b, ok := obj.Type().Underlying().(*types.Basic); ok {
		return b.Info()
	}
	return 0
}

func isBasicType(name string) bool {
	return basicInfo(name)&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) != 0
}

// ftype is a resolved metadata-column type: a basic type, a reference to
// another generated enum, a Go struct from the target package, or a slice of
// any of these.
type ftype struct {
	slice bool
	elem  string      // basic type name, referenced enum's type name, or struct name
	ref   *regEntry   // set when elem names a generated enum
	strct *structType // set when elem names a struct in the target package
}

func (t ftype) goType() string {
	if t.slice {
		return "[]" + t.elem
	}
	return t.elem
}

func (t ftype) tsType() string {
	base := "string"
	switch {
	case t.ref != nil:
		base = t.ref.spec.Type
	case t.elem == "bool":
		base = "boolean"
	case basicInfo(t.elem)&(types.IsInteger|types.IsFloat) != 0:
		base = "number"
	}
	if t.slice {
		return base + "[]"
	}
	return base
}

// field is one metadata column shared across the enum's members.
type field struct {
	Key      string // yaml key
	GoName   string // exported Go field/method name
	Inferred string // Go type inferred from the values (fallback)
	Type     ftype  // resolved type (declared or inferred)
}

// entry is one enum member.
type entry struct {
	Name     string
	Wire     string // literal for the const value (unquoted; quoted at render)
	Sentinel bool   // const-only boundary: excluded from tables and iteration
	Nodes    map[string]*yaml.Node
}

// regEntry is one enum in the cross-spec registry, used to resolve and validate
// references from other specs' fields.
type regEntry struct {
	spec      *spec
	exported  string          // pascal(type)
	container string          // container var name
	names     map[string]bool // declared member names, for reference validation
	tsOutAbs  string          // resolved absolute TS output path, or ""
}

func (r *regEntry) has(name string) bool {
	return r.names[name]
}

// goConst is the exported Go constant for a member.
func (r *regEntry) goConst(name string) string {
	return r.exported + r.spec.memberIdent(name)
}

// tsRef is the container-qualified TypeScript accessor for a member.
func (r *regEntry) tsRef(name string) string {
	return r.container + "." + r.spec.memberIdent(name)
}

// tsImportSpec is the module specifier another TypeScript file (at fromFile)
// uses to import this enum's symbols: an explicit tsModule if set, otherwise the
// path of its tsOut relative to fromFile (extension stripped, "./"-anchored).
func (r *regEntry) tsImportSpec(fromFile string) (string, error) {
	if r.spec.TSModule != "" {
		return r.spec.TSModule, nil
	}
	if r.tsOutAbs == "" {
		return "", fmt.Errorf("enum %s emits no TypeScript (set tsOut or tsModule on it to reference it from another TS file)", r.spec.Type)
	}
	rel, err := filepath.Rel(filepath.Dir(fromFile), r.tsOutAbs)
	if err != nil {
		return "", err
	}
	rel = strings.TrimSuffix(filepath.ToSlash(rel), path.Ext(rel))
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel, nil
}

// registry maps an enum type name to its entry.
type registry map[string]*regEntry

// parsed is one spec after structural parsing, before type resolution.
type parsed struct {
	path     string
	tsOutAbs string // resolved absolute TS output path, or ""
	spec     *spec
	fields   []field
	entries  []entry
}

// parse reads and validates an enum spec, filling defaults. Field types are
// still inferred here; declared-type resolution happens later against the
// registry (see resolveFields).
func parse(raw []byte) (*parsed, error) {
	var s spec
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.Package == "" || s.Type == "" {
		return nil, fmt.Errorf("package and type are required")
	}
	if s.Underlying == "" {
		s.Underlying = "int"
	}
	if info := s.underlyingInfo(); info&(types.IsInteger|types.IsFloat|types.IsString) == 0 {
		return nil, fmt.Errorf("underlying %q is not a predeclared integer, float, or string type", s.Underlying)
	}
	if s.Container == nil {
		c := s.Type + "s"
		s.Container = &c
	}
	switch s.Case {
	case "", "pascal", "snake", "preserve", "camel", "lower":
	default:
		return nil, fmt.Errorf("case %q is not one of pascal, snake, preserve, camel, lower", s.Case)
	}

	fields, entries, err := parseValues(s.Values)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Wire != "" {
			continue
		}
		if s.stringUnderlying() {
			entries[i].Wire = entries[i].Name
		} else {
			entries[i].Wire = strconv.Itoa(i)
		}
	}
	return &parsed{spec: &s, fields: fields, entries: entries}, nil
}

// buildRegistry indexes every parsed spec by its type name so references can be
// resolved and validated across the whole generation set.
func buildRegistry(all []*parsed) (registry, error) {
	reg := registry{}
	for _, p := range all {
		if _, dup := reg[p.spec.Type]; dup {
			return nil, fmt.Errorf("duplicate enum type %q", p.spec.Type)
		}
		names := make(map[string]bool, len(p.entries))
		for _, e := range p.entries {
			names[e.Name] = true
		}
		reg[p.spec.Type] = &regEntry{
			spec:      p.spec,
			exported:  pascal(p.spec.Type),
			container: *p.spec.Container,
			names:     names,
			tsOutAbs:  p.tsOutAbs,
		}
	}
	return reg, nil
}

// resolveFields assigns each column its final type: the declared type from the
// spec's fields block when present, else the inferred type.
func resolveFields(p *parsed, reg registry) error {
	for i := range p.fields {
		f := &p.fields[i]
		if decl, ok := p.spec.Fields[f.Key]; ok {
			t, err := parseTypeExpr(decl, reg)
			if err != nil {
				return fmt.Errorf("field %q: %w", f.Key, err)
			}
			f.Type = t
			continue
		}
		f.Type = inferredToFtype(f.Inferred)
	}
	return nil
}

// parseTypeExpr resolves a declared type expression to an ftype.
func parseTypeExpr(expr string, reg registry) (ftype, error) {
	expr = strings.TrimSpace(expr)
	var t ftype
	if rest, ok := strings.CutPrefix(expr, "[]"); ok {
		t.slice = true
		expr = strings.TrimSpace(rest)
	}
	t.elem = expr
	switch {
	case isBasicType(expr):
	case reg[expr] != nil:
		t.ref = reg[expr]
	default:
		// A named type that is neither basic nor a generated enum: treated as a
		// pending struct reference, resolved against the target package later
		// (see resolveStructRefs). An unresolvable name errors there.
	}
	return t, nil
}

// pendingStruct reports whether the type is a named reference still awaiting
// struct resolution against the target package.
func (t ftype) pendingStruct() bool {
	return t.ref == nil && t.strct == nil && !isBasicType(t.elem)
}

func inferredToFtype(inferred string) ftype {
	if rest, ok := strings.CutPrefix(inferred, "[]"); ok {
		return ftype{slice: true, elem: rest}
	}
	if inferred == "" {
		return ftype{elem: "string"}
	}
	return ftype{elem: inferred}
}

// memberIdent converts an entry name to its exported identifier, honoring Case.
// The default (pascal) suits camelCase names; "snake"/"preserve" keeps the name
// verbatim so UPPER_SNAKE members stay readable and can't collide under pascal.
func (s *spec) memberIdent(name string) string {
	switch s.Case {
	case "snake", "preserve":
		return name
	case "camel":
		return camel(name)
	case "lower":
		return strings.ToLower(name)
	default:
		return pascal(name)
	}
}

// parseValues does two passes: infer the field set + types, then keep raw nodes
// so literals can be rendered against the final (possibly widened) type.
func parseValues(nodes []yaml.Node) ([]field, []entry, error) {
	var fields []field
	idx := map[string]int{}
	var entries []entry

	for i := range nodes {
		n := &nodes[i]
		if n.Kind != yaml.MappingNode {
			return nil, nil, fmt.Errorf("values[%d]: expected a mapping", i)
		}
		e := entry{Nodes: map[string]*yaml.Node{}}
		for j := 0; j+1 < len(n.Content); j += 2 {
			key, val := n.Content[j].Value, n.Content[j+1]
			switch key {
			case "name":
				e.Name = val.Value
			case "wire":
				e.Wire = val.Value
			case "sentinel":
				e.Sentinel = val.Value == "true"
			default:
				e.Nodes[key] = val
				t := inferType(val)
				if p, ok := idx[key]; ok {
					fields[p].Inferred = merge(fields[p].Inferred, t)
				} else if t != "" || val.Kind == yaml.MappingNode {
					// A mapping infers "" but still declares a column (a struct
					// scalar); a bare null does not, so it can be pinned later.
					idx[key] = len(fields)
					fields = append(fields, field{Key: key, GoName: pascal(key), Inferred: t})
				}
			}
		}
		if e.Name == "" {
			return nil, nil, fmt.Errorf("values[%d]: missing name", i)
		}
		entries = append(entries, e)
	}
	return fields, entries, nil
}

func inferType(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "[]string"
	case yaml.MappingNode:
		return "" // a struct value; its type must be declared in fields
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!bool":
			return "bool"
		case "!!int":
			return "int"
		case "!!float":
			return "float64"
		case "!!str":
			return "string"
		case "!!null":
			return "" // unknown; a later value may pin it down
		default:
			log.Printf("inferType: unhandled scalar tag %q, defaulting to string", n.Tag)
		}
	default:
		log.Printf("inferType: unhandled node kind %d, defaulting to string", n.Kind)
	}
	return "string"
}

// merge widens conflicting inferences. Anything truly mixed degrades to string.
func merge(a, b string) string {
	switch {
	case a == b || b == "":
		return a
	case a == "":
		return b
	case (a == "int" && b == "float64") || (a == "float64" && b == "int"):
		return "float64"
	default:
		return "string"
	}
}

// iteratorFields resolves the spec's iterator column names to their fields, in
// declaration order, erroring on a name that is not a metadata column.
func iteratorFields(s *spec, fields []field) ([]field, error) {
	if len(s.Iterators) == 0 {
		return nil, nil
	}
	byKey := make(map[string]field, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}
	out := make([]field, 0, len(s.Iterators))
	for _, key := range s.Iterators {
		f, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("iterators: %q is not a metadata column", key)
		}
		out = append(out, f)
	}
	return out, nil
}

// liveEntries returns the members that participate in tables and iteration,
// dropping const-only sentinels.
func liveEntries(entries []entry) []entry {
	out := make([]entry, 0, len(entries))
	for _, e := range entries {
		if !e.Sentinel {
			out = append(out, e)
		}
	}
	return out
}

// wireLit renders a member's wire value as a literal of the underlying type,
// quoting it for string enums. The output is valid in both Go and TypeScript.
func wireLit(s *spec, wire string) string {
	if s.stringUnderlying() {
		return strconv.Quote(wire)
	}
	return wire
}

// basicLit renders a present scalar value as a Go/TS literal of a basic type.
func basicLit(elem string, n *yaml.Node) string {
	switch {
	case elem == "string":
		return strconv.Quote(n.Value)
	case elem == "bool":
		return n.Value
	case basicInfo(elem)&(types.IsInteger|types.IsFloat) != 0:
		return n.Value
	default:
		return strconv.Quote(n.Value)
	}
}

// basicZero is the Go zero literal for a basic type.
func basicZero(elem string) string {
	switch {
	case elem == "string":
		return `""`
	case elem == "bool":
		return "false"
	default:
		return "0"
	}
}

// goValue renders a field value as a Go literal of its resolved type. The
// second result is false when the value is absent and the field should be
// omitted from the composite literal (Go zero-fills references and slices).
func goValue(t ftype, n *yaml.Node) (string, bool, error) {
	absent := n == nil || n.Tag == "!!null"
	switch {
	case t.slice:
		if absent {
			return "", false, nil
		}
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := goScalar(t, c)
			if err != nil {
				return "", false, err
			}
			parts = append(parts, v)
		}
		return t.goType() + "{" + strings.Join(parts, ", ") + "}", true, nil
	case t.ref != nil || t.strct != nil:
		if absent {
			return "", false, nil
		}
		v, err := goScalar(t, n)
		return v, err == nil, err
	default:
		if absent {
			return basicZero(t.elem), true, nil
		}
		return basicLit(t.elem, n), true, nil
	}
}

// goScalar renders one scalar value: a basic literal, a validated enum const,
// or a struct composite literal.
func goScalar(t ftype, n *yaml.Node) (string, error) {
	switch {
	case t.strct != nil:
		r := &structRenderer{pkg: t.strct.pkg}
		return r.lit(t.strct.st, t.strct.name, n)
	case t.ref != nil:
		if !t.ref.has(n.Value) {
			return "", fmt.Errorf("%q is not a member of %s", n.Value, t.ref.spec.Type)
		}
		return t.ref.goConst(n.Value), nil
	default:
		return basicLit(t.elem, n), nil
	}
}
