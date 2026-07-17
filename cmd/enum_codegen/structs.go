package main

import (
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// structType is a Go struct named by a metadata column, resolved from the
// target package's real type information so nested literals render correctly.
type structType struct {
	name string
	st   *types.Struct
	pkg  *types.Package
}

// typeLoader loads and caches a package's type information by directory. A
// package is only loaded when a spec actually references a struct type, so
// enum-only generation never pays the cost.
type typeLoader struct {
	mu    sync.Mutex
	cache map[string]*types.Package
}

func newTypeLoader() *typeLoader {
	return &typeLoader{cache: map[string]*types.Package{}}
}

func (l *typeLoader) load(dir string) (*types.Package, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if p, ok := l.cache[dir]; ok {
		return p, nil
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 || pkgs[0].Types == nil {
		return nil, fmt.Errorf("no package types loaded from %s", dir)
	}
	l.cache[dir] = pkgs[0].Types
	return pkgs[0].Types, nil
}

// lookupStruct finds a named struct type by name in the loaded package.
func lookupStruct(pkg *types.Package, name string) (*structType, error) {
	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		return nil, fmt.Errorf("type %q not found in package %s", name, pkg.Name())
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%q is not a named type", name)
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%q is not a struct", name)
	}
	return &structType{name: name, st: st, pkg: pkg}, nil
}

// structRenderer renders yaml values as Go literals against real struct types.
type structRenderer struct {
	pkg *types.Package
}

// lit renders a yaml mapping as a composite literal of a struct.
func (r *structRenderer) lit(st *types.Struct, name string, n *yaml.Node) (string, error) {
	if n.Kind != yaml.MappingNode {
		return "", fmt.Errorf("expected a mapping for %s", name)
	}
	var parts []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1]
		f, err := structField(st, key)
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		v, err := r.value(f.Type(), val)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", name, key, err)
		}
		parts = append(parts, f.Name()+": "+v)
	}
	return name + "{" + strings.Join(parts, ", ") + "}", nil
}

// value renders a yaml node as a Go literal of the given type.
func (r *structRenderer) value(t types.Type, n *yaml.Node) (string, error) {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Info()&types.IsString != 0 {
			return strconv.Quote(n.Value), nil
		}
		return n.Value, nil
	case *types.Slice:
		if n.Kind != yaml.SequenceNode {
			return "", fmt.Errorf("expected a sequence")
		}
		name, err := r.typeName(t)
		if err != nil {
			return "", err
		}
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := r.value(u.Elem(), c)
			if err != nil {
				return "", err
			}
			parts = append(parts, v)
		}
		return name + "{" + strings.Join(parts, ", ") + "}", nil
	case *types.Struct:
		name, err := r.typeName(t)
		if err != nil {
			return "", err
		}
		return r.lit(u, name, n)
	case *types.Pointer:
		v, err := r.value(u.Elem(), n)
		if err != nil {
			return "", err
		}
		return "&" + v, nil
	default:
		return "", fmt.Errorf("unsupported field type %s", t)
	}
}

// typeName renders a type as it should appear in generated source. Types from
// other packages are rejected, since emitting them would need imports that are
// not wired up.
func (r *structRenderer) typeName(t types.Type) (string, error) {
	switch u := t.(type) {
	case *types.Basic:
		return u.Name(), nil
	case *types.Slice:
		e, err := r.typeName(u.Elem())
		if err != nil {
			return "", err
		}
		return "[]" + e, nil
	case *types.Pointer:
		e, err := r.typeName(u.Elem())
		if err != nil {
			return "", err
		}
		return "*" + e, nil
	case *types.Named:
		if p := u.Obj().Pkg(); p != nil && p != r.pkg {
			return "", fmt.Errorf("type %s is from another package (imports not supported)", u.Obj().Name())
		}
		return u.Obj().Name(), nil
	default:
		return "", fmt.Errorf("unsupported type %s", t)
	}
}

// structField resolves a yaml key to a struct field by json tag then field name.
func structField(st *types.Struct, key string) (*types.Var, error) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		if name, _, _ := strings.Cut(tag, ","); name != "" && name == key {
			return f, nil
		}
		if strings.EqualFold(f.Name(), key) {
			return f, nil
		}
	}
	return nil, fmt.Errorf("no field for key %q", key)
}
