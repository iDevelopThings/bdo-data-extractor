# Extending the extractor

Guidelines for agents (and humans) adding or changing data sources in
`bdo-data-extractor`. Coding style, reuse rules, and general philosophy live in
local `AGENTS.md` (gitignored). Byte-level format notes live in
**[FORMATS.md](FORMATS.md)**. This doc is the **how to wire a new source** map.

The JSON output is part of the public contract — treat shape changes like API
breaks and record them in **[CHANGELOG.md](CHANGELOG.md)**.

**Performance is also a contract.** A full `build` is already tuned to a few
seconds; careless new passes (extra PAZ reads, mid-stage encodes, full loc
decodes) can double that. Read the **Performance** section before adding a
stage or a hot-path join.

---

## Architecture (where code goes)

```
main.go / pipeline.RunAll
        │
        ▼
internal/build          assemble stages → register JSON artifacts
        │
        ├── internal/paz      read PAZ archives (basename only)
        ├── internal/tables   decode one .bss/.dbss → typed rows
        ├── internal/bss      Cursor, offset-index, PABR, UTF-16
        ├── internal/loc      languagedata_*.loc → GameStrings
        └── src/model         output types + URNs
                │
                ▼
        src/output            transactional publish → out dir
```

| Package | Responsibility |
|---------|----------------|
| `internal/paz` | Archive index + decompress. Concurrent-safe `Read` / `Find`. |
| `internal/bss` | Shared binary helpers. Extend here; do not fork readers in tables. |
| `internal/tables` | Leaf decoders. IDs and raw fields — not viewer models. |
| `internal/loc` | One-pass `LoadGame` into `GameStrings`. |
| `internal/build` | Join tables + loc → `src/model`, register outputs. |
| `src/model` / `src/urn` | Emitted JSON contract. |
| `src/output` | Staging + atomic publish (see `TRANSACTIONAL_OUTPUT_SPEC.md`). |
| `pipeline` | Embedder entry: build + icons/worldmap + `manifest.json`. |

**Do not revive** the removed `internal/schema` package or a `table` CLI
subcommand. Decoders are hand-written in `internal/tables`.

---

## Checklist: add a new data source

Work top-down only after you know the wire layout (FORMATS + live PAZ probe).
A recent small example is item improvements:
`internal/tables/itemimprovement.go` → `internal/build/itemimprovements.go`.

1. **Read from PAZ** — basename only via `b.src.Read("foo.dbss")` or sibling
   pairs via `b.readFiles("foooffset.dbss", "foo.dbss")`
   (`internal/build/read.go`). Never invent paths under `F:\BDOData`.
2. **Decode in `internal/tables`** — use `bss` helpers (below). Return
   tables-layer types (keys, counts, raw strings), not `src/model` refs.
3. **Unit-test the decoder** — synthetic bytes (`PackPABR`, little-endian
   buffers). Optional live fixtures under `.tmp/` with `t.Skip` when absent —
   do not commit game blobs.
4. **Loc (if needed)** — extend `GameStrings` + `LoadGame` in
   `internal/loc/loc.go` (new `key0` constant, field, switch branch). Prefer
   shared text structs over parallel maps.
5. **Build stage** — `internal/build/<name>.go`: join to `b.items` / `b.gs` /
   other stage state, map to `src/model`, call `b.addJSON` (or
   `addExclusiveOutput` for huge arrays).
6. **Wire the stage** — either append to the stage list in
   `(*Builder).Run` (`internal/build/build.go`) or call from an existing
   stage when the data is a sidecar of that stage (e.g. item improvements
   from `buildItems`).
7. **Cross-file links** — use `models.EntityRef` / `EntityRefList` and `urn`
   domains; do not invent ad-hoc `{id,name}` blobs when a ref type exists.
8. **Enums** — add YAML under `src/model/enums/` and regenerate via
   `cmd/enum_codegen` (see its README).
9. **Docs** — README output table row, FORMATS section if the layout is new,
   CHANGELOG entry (call out JSON shape changes).
10. **Verify** — `go build -buildvcs=false ./...`, `go vet -buildvcs=false ./...`,
    `go test ./... -race`, and a smoke `build --out=.tmp/...` when the JSON
    contract changed. Compare with `diff-outputs` against a baseline dump when
    string policy or joins might drift. Check `[stage]` / `[done]` timings if
    you touched items, world, loc, or publish — see **Performance**.

Prefer attaching to an existing stage when the artifact is tightly coupled
(item sidecars). Prefer a new top-level stage when later stages must consume
the result independently.

---

## BSS decode idioms

### Prefer these

| Helper | Use when |
|--------|----------|
| `bss.IndexedRecords(offset, data)` | Standard 12-byte key/offset/size index |
| `bss.IndexedRecordsU16(name, …)` | Compact u16-key indexes |
| `bss.RecordsFromEntries(entries, data)` | Index already parsed/validated |
| `bss.NewCursor` + typed reads | Sequential field walk inside a record |
| `bss.RequireExhausted(c)` | Fixed layout fully consumed (trunc + trailing bytes) |
| `bss.DecodeUTF16` | UTF-16LE blobs (loc and inline strings) |
| `bss.OpenPABR` + string-table helpers | Real PABR tables with footer string table |
| `bss.PABRCount` | PABR-**framed sidecars** (BKD/RID) that are **not** full tables |
| `bss.PackPABR` | Tests only — synthesize PABR fixtures |
| `b.readFiles(names...)` | Parallel sibling PAZ reads |

Canonical loop:

```go
for rec, err := range bss.IndexedRecords(offsetRaw, data) {
	if err != nil {
		return nil, err
	}
	c := bss.NewCursor(rec.Data)
	// … read fields …
	if err := bss.RequireExhausted(c); err != nil {
		return nil, fmt.Errorf("key %d: %w", rec.Entry.Key, err)
	}
}
```

Size-branching or type-conditional rows may still parse the index manually and
`entry.Slice` when not every key shares one layout — document why in a short
comment and keep unknowns when that table's style preserves them.

### Avoid these

- Calling `OpenPABR` on BKD/RID-style sidecars — use `PABRCount`.
- Hand-rolling LE u32 / UTF-16 readers beside `bss`.
- Swallowing a missing **required** table (`Read` must error).
- Treating a **present but corrupt** optional table as empty — if the stage
  runs, decode errors fail the build. `ReadIfExists`-style skips are only for
  **documented whole optional stages** (absent file → skip stage).
- Mid-stage JSON writes — register artifacts; `publishOutputs` owns the disk.

FORMATS §2 is the oracle for PABR vs offset-index vs hybrid layouts.

---

## Localization (`GameStrings`)

- Loc files live under the **game install** (`ads/languagedata_<lang>.loc`), not
  PAZ. `loc.LoadGame` walks records once and fills only the tables the build
  needs.
- **Adding a table**: new `key0` constant, `GameStrings` field with a
  scan-friendly comment (table number + key shape), decode via
  `decodeLocString`, assign with a **named** helper (`setNameDesc`,
  `setItemField`, …) — not boolean “cleanDesc” flags.
- **Shared types**: `Text`, `ItemText`, `KnowledgeCardText`, `EntityText`,
  `TerritoryText`. Use special structs only when columns are not a name/desc
  pair (quests, market cats, jewel groups, adventure journals).
- **PA markup**: keep `<PAColor0xAARRGGBB>` and `<PAOldColor>`. Do not strip for
  storage. Strip only in parse helpers that need plain `+N` text
  (`ParseStatFromLoc`).
- **Buffs (table 5)**: store the **full** multi-line blob in `BuffNames`
  (first-write-wins on duplicate ids). Do not truncate to the first line.

Korean source names often come from PABR string tables; localized display
names come from loc. Join carefully (trim is intentional for some entity
name matches — see loc helpers).

---

## Build stages and outputs

Stages **register only**. Nothing replaces the live output directory until
`publishOutputs` → `Transaction.Publish()`.

- `b.addJSON(name, value)` — normal artifacts.
- `b.addExclusiveOutput` + `output.NewJSONArray` — huge arrays (`items.json`,
  `item_enhancements.json`).
- Ownership is recorded in `.build-outputs.json`. The next publish replaces or
  removes **only** builder-owned JSON. Icons, worldmap tiles, and
  `manifest.json` are outside that set unless registered.
- `pipeline.Options.SkipAssets` skips icons + worldmap (~1GB) for timing A/B;
  CLI `build` never writes those assets.

Full crash/recovery rules: `src/output/TRANSACTIONAL_OUTPUT_SPEC.md`.

Cross-stage state lives on `Builder` (items, regions, memoized tables). If a
later stage needs an earlier decode, memoize on `Builder` rather than reading
the PAZ twice — see `npcsDecoded`, `nodesDecoded`, `characterFunction*`.

---

## Performance (critical)

A full `build` already finishes in a handful of seconds for ~72k items. The
pipeline has been through several perf passes (parallel PAZ reads, deferred
parallel publish, IndexedRecords, bounded writers). **New work must not casually
undo that.** A “simple” serial re-read or mid-stage JSON write can add seconds;
doing it in a hot loop can blow the run.

Treat wall time as a regression surface the same way you treat JSON shape.

### What already keeps it fast

- **One pass per concern** — loc fills only the `key0` tables the build needs;
  stages register artifacts; **one** transactional publish at the end (parallel
  writers), not per-stage disk I/O.
- **Sibling PAZ reads** via `b.readFiles` — offset+data (and related) files load
  concurrently. Prefer that over sequential `Read` calls when you need multiple
  basenames together.
- **Memoized heavy tables on `Builder`** — decode once, reuse across stages
  (`npcsDecoded`, exploration/waypoint tables, `characterFunction*`, …).
- **Bounded fan-out** with `parallel.For` for independent records (item decode,
  recipe XMLs, icons). `paz.Source` is concurrent-safe.
- **Assets are separate** — icons + worldmap are ~1GB and dominate
  `pipeline.RunAll`. CLI `build` does not write them; use
  `pipeline.Options.SkipAssets` for JSON-only timing A/B.

Hot stages today tend to be **items** (loadTables + merge), **write outputs**
(especially `--pretty`), and **world**. Watch `[stage]` / `[done]` lines (and
`[step]` inside items) when you change those paths.

### Rules when extending

1. **Do not re-read or re-decode** a PAZ table another stage already loaded —
   memoize on `Builder` or take data from an earlier stage’s result.
2. **Do not `Read` inside a per-item / per-row loop.** Batch with `readFiles` or
   decode once into a map, then join.
3. **Do not write JSON until publish.** Mid-stage encodes were a major historical
   cost; keep registration cheap.
4. **Do not decode the entire loc file** for one new string table — add a `key0`
   branch to `LoadGame` only.
5. **Do not scan all item XMLs twice** without a reason — `scanItemInfo` /
   recipe parsing already touch that corpus; fold new needs into an existing
   pass when possible.
6. **Pretty JSON is for humans/diffs**, not default perf. Compact build is the
   speed baseline; `--pretty` can cost multiple seconds on write alone.
7. **Profile before “optimizing” new code** — `--cpuprofile`, stage timings, and
   (locally) `scripts/bench-build.ps1` / DecodeItemStats benches. Fix the hot
   path you measured, not a guessed one.
8. **Embedders**: call `build.Release()` (and rely on `pipeline`’s
   `FreeOSMemory`) after a run so the next extract doesn’t keep hundred-MB maps
   alive.

### When a change might regress

If you add a stage, a large join, or touch items/world/loc/publish:

- Smoke-build to `.tmp/…` and compare `[done]` totals to a known good run.
- For string/join policy changes, baseline + `diff-outputs` (correctness) and
  glance at stage timings (speed).
- Prefer extending an existing parallel path over introducing a new serial
  walk of items or the archive listing.

A correct but 30s extract is a bug for this project’s bar. Ask before adding
dependencies or abstractions that trade clarity for extra passes over the
dataset.

---

## Parallelism

Prefer the existing **`parallel`** package (`github.com/dgravesa/go-parallel`):

- `b.readFiles` for sibling PAZ reads.
- `parallel.For` for independent record/decode fan-out (items, recipes, icons).

Do not invent ad-hoc worker pools for table work. Bound output writers through
`src/output`. See **Performance** above for when fan-out is required vs harmful
(don’t parallelize tiny work; do parallelize independent PAZ/record batches).

---

## Reverse-engineering sources

Hard rules (also in `AGENTS.md`):

- **Never** use `F:\BDOData` (or other stale extracts) as a binary schema
  oracle. Read live client files through `internal/paz`.
- XML/Lua from old extracts are fine as **hints**; logic must run on PAZ
  (or game-dir loc) data.
- Do not write into the game install — `AssertSafeOut` refuses that.
- Keep probes and benches under gitignored `scripts/` and `.tmp/` — never
  commit them or raw `.bss`/`.dbss`/`.loc` dumps.

For cracking a new table layout, use the local reverse-engineering skill /
FORMATS gotchas: validate against the live client, not memory of a previous
extract. When string policy or joins change, baseline a pretty build and
`diff-outputs` so regressions are file+id bisectable.

---

## Testing expectations

| Layer | Pattern |
|-------|---------|
| `internal/tables` | Synthetic records; assert keys, sizes, `RequireExhausted` failures |
| `internal/build` | Fake `Builder{gs: …}` for join helpers — no full PAZ required |
| `internal/loc` | Helper unit tests for setters / lightstone name derivation |
| `src/output` | Transaction crash/recovery cases |

Always run tests with **`-race`**. Format with `gofmt`. Ask before adding a
new third-party dependency.

---

## Failure modes (do not repeat)

1. Re-deriving a helper that already lives in `bss` / `loc` / `jsonio`.
2. Using stale extracted binaries as layout truth.
3. Stripping PA tags or truncating buff loc “to clean up” display.
4. Silent empty maps when a required table is missing or corrupt.
5. Writing JSON from the middle of a stage.
6. Committing `scripts/`, `.tmp/`, or game file dumps.
7. Premature abstractions (interfaces with one impl, generic soup for one call).
8. Narrative comments (“now we…”, “changed from…”) — describe current code only.
9. Assuming every offset-index table is fixed-size — many are type-conditional
   (see FORMATS gotchas / `itemenchant`).
10. Re-reading PAZ tables or scanning item XMLs again instead of memoizing /
    folding into an existing pass (silent multi-second regressions).
11. Serial `Read` loops or per-row archive access where `readFiles` /
    `parallel.For` already exist for that pattern.

---

## Related docs

| Doc | Owns |
|-----|------|
| [README.md](README.md) | Install, CLI, output catalog |
| [FORMATS.md](FORMATS.md) | Wire layouts and per-table notes |
| [CHANGELOG.md](CHANGELOG.md) | User-visible / contract history |
| [src/output/TRANSACTIONAL_OUTPUT_SPEC.md](src/output/TRANSACTIONAL_OUTPUT_SPEC.md) | Publish algorithm |
| [cmd/enum_codegen/README.md](cmd/enum_codegen/README.md) | Enum YAML → Go/TS |
| Local `AGENTS.md` | Style, philosophy, commit hygiene |
