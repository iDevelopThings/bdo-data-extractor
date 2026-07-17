// Command enum_codegen generates container-based Go enums (and, optionally,
// matching TypeScript types) from YAML specs.
//
// Each spec declares a type, its underlying representation, and its members
// with per-member metadata. Output locations may be set on the spec itself
// (out / tsOut, resolved relative to the spec file), so a //go:generate
// directive only needs to point at the spec or a directory of specs:
//
//	//go:generate go run ../../cmd/enum_codegen -in ./enums
//
// The -out / -root flags override the spec's out for one-off runs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	in := flag.String("in", ".", "yaml file or directory of *.yml specs")
	out := flag.String("out", "", "write generated Go to this exact directory (overrides the spec's out)")
	root := flag.String("root", "", "write to <root>/<package>; for a central spec dir fanning out per package")
	flag.Parse()

	if *out != "" && *root != "" {
		check(errors.New("-out and -root are mutually exclusive"))
	}

	files, err := collect(*in)
	check(err)
	if len(files) == 0 {
		check(fmt.Errorf("no yaml specs found in %s", *in))
	}

	// Parse every spec first so cross-spec references resolve against the whole
	// set, then resolve field types, then render.
	all := make([]*parsed, 0, len(files))
	for _, f := range files {
		p, err := parse(mustRead(f))
		if err != nil {
			check(fmt.Errorf("%s: %w", f, err))
		}
		p.path = f
		p.tsOutAbs = tsOutPath(f, p.spec.TSOut)
		all = append(all, p)
	}
	reg, err := buildRegistry(all)
	check(err)
	loader := newTypeLoader()
	for _, p := range all {
		if err := resolveFields(p, reg); err != nil {
			check(fmt.Errorf("%s: %w", p.path, err))
		}
		if err := resolveStructRefs(p, goOutDir(p.path, p.spec, *out, *root), loader); err != nil {
			check(fmt.Errorf("%s: %w", p.path, err))
		}
	}
	for _, p := range all {
		check(gen(p, *out, *root))
	}
}

// resolveStructRefs resolves any column whose declared type is a named struct in
// the target package, loading that package's types on demand.
func resolveStructRefs(p *parsed, dir string, loader *typeLoader) error {
	for i := range p.fields {
		f := &p.fields[i]
		if !f.Type.pendingStruct() {
			continue
		}
		pkg, err := loader.load(dir)
		if err != nil {
			return fmt.Errorf("loading package types from %s: %w", dir, err)
		}
		st, err := lookupStruct(pkg, f.Type.elem)
		if err != nil {
			return fmt.Errorf("field %q: %w", f.Key, err)
		}
		f.Type.strct = st
	}
	return nil
}

func mustRead(path string) []byte {
	raw, err := os.ReadFile(path)
	check(err)
	return raw
}

func collect(in string) ([]string, error) {
	fi, err := os.Stat(in)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return []string{in}, nil
	}
	a, err := filepath.Glob(filepath.Join(in, "*.yaml"))
	if err != nil {
		return nil, err
	}
	b, err := filepath.Glob(filepath.Join(in, "*.yml"))
	if err != nil {
		return nil, err
	}
	return append(a, b...), nil
}

func gen(p *parsed, out, root string) error {
	path, s := p.path, p.spec

	srcName := filepath.Base(path)
	src, err := renderGo(s, p.fields, p.entries, srcName)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	goDir := goOutDir(path, s, out, root)
	if err := ensureDir(goDir, filepath.Dir(path)); err != nil {
		return err
	}
	goDst := filepath.Join(goDir, snake(s.Type)+"_enum.go")
	fmt.Fprintf(os.Stderr, "enum_codegen: %s -> %s (%d values)\n", path, goDst, len(p.entries))
	if err := os.WriteFile(goDst, src, 0o644); err != nil {
		return err
	}

	if p.tsOutAbs == "" {
		return nil
	}
	// The TS target often lives in a separate repo (e.g. a viewer alongside this
	// one). Never create its directory tree: writing into a missing path would
	// scatter stray dirs/files for anyone using this package without that repo.
	// Require the directory to exist; otherwise warn and skip.
	tsDir := filepath.Dir(p.tsOutAbs)
	if info, err := os.Stat(tsDir); err != nil || !info.IsDir() {
		if err == nil || os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "enum_codegen: skipping TypeScript for %s: output directory %s does not exist\n", s.Type, tsDir)
			return nil
		}
		return err
	}
	tsSrc, err := renderTS(s, p.fields, p.entries, srcName, p.tsOutAbs)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "enum_codegen: %s -> %s\n", path, p.tsOutAbs)
	return os.WriteFile(p.tsOutAbs, tsSrc, 0o644)
}

// tsOutPath resolves a spec's TS output to an absolute path (relative to the
// spec file), or "" when the spec emits no TypeScript.
func tsOutPath(specPath, tsOut string) string {
	if tsOut == "" {
		return ""
	}
	if filepath.IsAbs(tsOut) {
		return tsOut
	}
	return filepath.Join(filepath.Dir(specPath), tsOut)
}

// goOutDir resolves the Go output directory: the -out / -root flags win, then
// the spec's out (relative to the spec file), else alongside the spec.
func goOutDir(path string, s *spec, out, root string) string {
	switch {
	case out != "":
		return out
	case root != "":
		return filepath.Join(root, s.Package)
	case s.Out != "":
		return filepath.Join(filepath.Dir(path), s.Out)
	default:
		return filepath.Dir(path)
	}
}

func ensureDir(dir, srcDir string) error {
	if dir == srcDir {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "enum_codegen:", err)
		os.Exit(1)
	}
}
