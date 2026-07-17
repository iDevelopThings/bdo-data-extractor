# enum_codegen

Generates container-based Go enums — and, optionally, matching TypeScript
types — from small YAML specs. One spec describes an enum type, its underlying
representation, and its members with arbitrary per-member metadata; the
generator emits the constants, lookup tables, and helper methods so callers get
name/value/metadata resolution without hand-maintaining parallel maps.

## Usage

Point `-in` at a single spec or a directory of specs. Output locations are set
on the spec itself (`out`, `tsOut`), so a `//go:generate` directive only needs
to name the input:

```go
//go:generate go run ../../cmd/enum_codegen -in ./enums
```

Then `go generate ./...`. See [`src/model/gen.go`](../../src/model/gen.go) and
the specs in [`src/model/enums`](../../src/model/enums).

Flags:

| flag    | meaning                                                                    |
| ------- | ------------------------------------------------------------------------- |
| `-in`   | a `.yml`/`.yaml` file, or a directory of them (default `.`)               |
| `-out`  | write the Go file to this exact directory, overriding the spec's `out`     |
| `-root` | write to `<root>/<package>`; for a central spec dir fanning out per package |

`-out` and `-root` are mutually exclusive.

## Spec format

```yaml
package: model          # required: Go package of the generated file
type: SlotName          # required: generated enum type name
underlying: byte        # optional: base type (default: int)
marshalText: false      # optional: emit MarshalText/UnmarshalText (default: false)
container: SlotNames    # optional: container var name (default: <type> + "s")
out: ..                 # optional: Go output dir, relative to THIS file
tsOut: ../ui/slots.ts   # optional: TypeScript output file, relative to THIS file
values:
  - name: MainWeapon    # required: member identifier (PascalCase)
    wire: 0             # optional: const value (default: index, or name for string enums)
    title: "Main Weapon" # any extra keys become typed metadata columns
    desc: "RightHand"
  - name: MAX
    wire: 47
    sentinel: true      # const-only boundary; excluded from tables and iteration
```

### `underlying`

Any predeclared integer (`byte`, `rune`, `int8`…`int64`, `uint8`…`uintptr`),
float (`float32`/`float64`), or `string` type. The type is classified via
`go/types`, so all integer widths are handled uniformly and the correct `fmt`
verb is chosen for the `String()` fallback. Anything else is rejected at
generation time.

For string enums, `wire` values are emitted as quoted string literals and
default to the member `name` when omitted; for numeric enums they default to the
declaration index.

### `wire`

The constant's underlying value. Gaps are fine (the numbers come from the game
client, not a dense sequence). Negative values are allowed for signed types.

### `sentinel`

A member marked `sentinel: true` is emitted as a named constant only — a
boundary such as `MAX` used for bounds/counts. It is deliberately excluded from
the container, the `Infos`/`Values`/`ByName` tables, iteration, and the derived
TypeScript union, so it never appears as a "real" value.

### Metadata columns

Every key on a member other than `name`/`wire`/`sentinel` becomes a metadata
column. Its Go type is inferred from the YAML scalar tag and widened across all
members:

| YAML          | Go type    |
| ------------- | ---------- |
| `!!int`       | `int`      |
| `!!float`     | `float64`  |
| `!!bool`      | `bool`     |
| `!!str`       | `string`   |
| sequence      | `[]string` |

(YAML carries no integer-width information, so integer metadata is always `int`
— distinct from `underlying`, which is the wire type you name explicitly.)

### Declaring column types (`fields`)

Inference is the fallback; a top-level `fields:` block overrides it per column
when you need a specific type. Anything not listed stays inferred.

```yaml
fields:
  stat: StatId        # reference to another generated enum
  stats: "[]StatId"   # slice of references  (quote []… — YAML reads it as a flow seq otherwise)
  tier: int64         # a specific integer width
  weights: "[]float64"
```

A type is one of: a predeclared basic type (`string`, `bool`, any integer width,
`float32`/`float64`), the **type name of another enum** in the same generation
set (a reference), the **name of a Go struct** in the target package, or a slice
of any of these.

**Enum references** are the main reason to declare a type. `stat: fishingSpeed`
typed as `StatId` emits the compile-checked constant `StatIdFishingSpeed` (Go)
and `StatIds.FishingSpeed` (TS), honoring the referenced enum's own `case` mode.
The generator resolves references across the whole `-in` set in a first pass, so
it **errors at generation time** if a value isn't a real member of the target
enum — which is how a typo or a stale name gets caught. A member that omits a
reference/slice column is left as the Go zero value (the property is omitted).

TypeScript references work both within an enum (self-references, e.g. `fanout`)
and across files. A **cross-file** reference emits an `import` for the target
enum's symbols and resolves values to its container (`StatIds.FishingSpeed`). The
import module is, in order of precedence:

1. the target enum's `tsModule`, if set — a verbatim specifier, e.g.
   `"@/model/stats"` (use this when the target's TS symbols are hand-written or
   behind a path alias, or it emits no TS of its own); otherwise
2. the path of the target's `tsOut` **relative to** this file's `tsOut`, with the
   extension stripped (`import { StatIds, type StatId } from "../stats.gen"`).

If the target has neither `tsOut` nor `tsModule`, the reference errors — there is
nothing to import. Only the container is imported as a value (and only when a
value actually uses it); the type is imported as `type`.

**Struct columns.** A column can be typed as an existing Go struct in the target
package (e.g. `derived: "[]EffectDsl"`). The generator loads that package with
`go/packages`, resolves the struct's real fields, and renders nested composite
literals against them — YAML keys map to fields by `json` tag (then field name),
and each value is rendered with the field's actual type (so `args: [10]` on an
`Args []float64` field emits `[]float64{10}`, not `[]int{...}`). Basic fields,
slices, pointers, and nested same-package structs are supported; a field from
another package is rejected (its import isn't wired up). Struct columns are
Go-only — a struct column in a `tsOut` enum errors. Package types are loaded
lazily, so enum-only generation never invokes `go/packages`.

### Extra iterators (`iterators`)

List metadata columns to expose as extra iterators over their values, saving a
hand-written side file:

```yaml
iterators: [title, alias]
```

For each column this emits a Go container method `Iter<Field>() iter.Seq[T]` and
a TypeScript generator `iter<Field>()`, both yielding each member's value in
declaration order (`T` is the column's type; an optional TS column yields
`... | undefined`). Get an array in TS with `[...iterTitle()]`. An `iterators`
entry that names a non-column errors.

## Generated Go API

For `type SlotName byte` the generator produces:

- Constants: `SlotNameMainWeapon`, … (including sentinels).
- `SlotNames` — a container struct value; field per member for typed access.
- `SlotNameInfo` — a struct embedding the enum plus `Name` and every metadata
  column.
- Container methods: `Values()`, `Infos()`, `Len()`, `All()` (an `iter.Seq`),
  `FromWire(byte)`, `Parse(string)`.
- Value methods: `Info()`, `TryGetInfo()`, `Valid()`, `Wire()`, `Name()`,
  `String()`, plus an accessor per metadata column (e.g. `Title()`, `Desc()`).
- With `marshalText: true`: `MarshalText`/`UnmarshalText`.

## Generated TypeScript

When `tsOut` is set, a parallel module is written with a `const` values object,
a derived union type (`type SlotName = (typeof SlotNames)[keyof typeof
SlotNames]`), an `Info` interface, and `Infos`/`Values`/`ByName` tables.
Sentinels are emitted as standalone consts and excluded from the union.

**The TS output directory is never created.** `tsOut` commonly points into a
separate repo (e.g. a viewer alongside this one), so the generator writes the
file only when its directory already exists; otherwise it prints a skip warning
and moves on. Go generation is unaffected. This keeps a consumer who uses this
package *without* that repo from having a stray external directory tree created —
they just get the Go enums. (Create the directory, or drop `tsOut`, to change
that.)

A metadata column that some members omit becomes an optional interface property
(`unit?: string`) and is left off those members' entries entirely, rather than
being filled with an empty string / empty array as the Go zero value requires.
Columns present on every member stay required.

`ByName` is keyed by the exact member name (unlike the Go side, whose unexported
lookup map is lowercased to back a case-insensitive `Parse`; TypeScript has no
lookup-time lowercasing). For the same case-insensitive resolution, a
`parse<Type>(name)` function is generated that trims and lowercases before
looking up, returning `undefined` for an unknown name.

## Layout

| file          | responsibility                                        |
| ------------- | ----------------------------------------------------- |
| `main.go`     | flags, spec collection, output-path resolution        |
| `spec.go`     | spec parsing, type inference, underlying classification |
| `gen_go.go`   | Go renderer                                            |
| `gen_ts.go`   | TypeScript renderer                                    |
| `naming.go`   | identifier case helpers                                |
