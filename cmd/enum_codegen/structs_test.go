package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempPkg creates a minimal loadable Go package defining a struct the
// generator can resolve via go/packages.
func writeTempPkg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module foo\n\ngo 1.24\n")
	write("foo.go", `package foo

type Rule struct {
	Func string    `+"`json:\"func\"`"+`
	Args []float64 `+"`json:\"args,omitempty\"`"+`
}
`)
	return dir
}

func TestRenderGoStructColumn(t *testing.T) {
	dir := writeTempPkg(t)

	const spec = `
package: foo
type: T
underlying: string
fields:
  rules: "[]Rule"
values:
  - name: A
    rules:
      - func: DO_THING
        args: [10, 20]
  - name: B
`
	p, err := parse([]byte(spec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	reg, err := buildRegistry([]*parsed{p})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if err := resolveFields(p, reg); err != nil {
		t.Fatalf("resolveFields: %v", err)
	}
	if err := resolveStructRefs(p, dir, newTypeLoader()); err != nil {
		t.Fatalf("resolveStructRefs: %v", err)
	}
	out, err := renderGo(p.spec, p.fields, p.entries, "t.yml")
	if err != nil {
		t.Fatalf("renderGo: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, "Rules []Rule") {
		t.Errorf("struct-slice field type missing\n---\n%s", got)
	}
	// json tags map func->Func, args->Args; []float64 rendered with the real type.
	if !strings.Contains(got, `Rule{Func: "DO_THING", Args: []float64{10, 20}}`) {
		t.Errorf("nested struct literal wrong\n---\n%s", got)
	}
	if !strings.Contains(got, "func (v T) Rules() []Rule") {
		t.Error("accessor should return the struct slice type")
	}
}

func TestResolveStructRefUnknownTypeErrors(t *testing.T) {
	dir := writeTempPkg(t)
	const spec = `
package: foo
type: T
underlying: string
fields:
  x: Nope
values:
  - name: A
    x: {}
`
	p, err := parse([]byte(spec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	reg, _ := buildRegistry([]*parsed{p})
	if err := resolveFields(p, reg); err != nil {
		t.Fatalf("resolveFields: %v", err)
	}
	err = resolveStructRefs(p, dir, newTypeLoader())
	if err == nil {
		t.Fatal("expected an error for an unknown struct type")
	}
	if !strings.Contains(err.Error(), "Nope") {
		t.Errorf("error should name the missing type, got: %v", err)
	}
}

func TestGenSkipsMissingTSDir(t *testing.T) {
	goDir := t.TempDir()
	p := build(t, `
package: p
type: T
underlying: string
values:
  - name: a
    label: "A"
`)[0]
	p.path = filepath.Join(goDir, "t.yml")
	p.tsOutAbs = filepath.Join(goDir, "does", "not", "exist", "t.gen.ts")

	if err := gen(p, goDir, ""); err != nil {
		t.Fatalf("gen should not fail when the TS dir is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(goDir, "t_enum.go")); err != nil {
		t.Errorf("Go output should still be written: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(p.tsOutAbs)); !os.IsNotExist(err) {
		t.Error("the TS output directory tree must not be created")
	}
	if _, err := os.Stat(p.tsOutAbs); !os.IsNotExist(err) {
		t.Error("TS output should be skipped, not written")
	}
}

func TestRenderTSRejectsStructColumn(t *testing.T) {
	dir := writeTempPkg(t)
	const spec = `
package: foo
type: T
underlying: string
tsOut: out.ts
fields:
  rules: "[]Rule"
values:
  - name: A
    rules:
      - func: X
`
	p, _ := parse([]byte(spec))
	reg, _ := buildRegistry([]*parsed{p})
	_ = resolveFields(p, reg)
	if err := resolveStructRefs(p, dir, newTypeLoader()); err != nil {
		t.Fatalf("resolveStructRefs: %v", err)
	}
	_, err := renderTS(p.spec, p.fields, p.entries, "t.yml", "")
	if err == nil {
		t.Fatal("expected TS rendering to reject a struct column")
	}
}
