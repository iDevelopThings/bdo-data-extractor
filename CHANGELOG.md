# Changelog

All notable changes to `bdo-data-extractor` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions are
[semantic](https://semver.org/), with one caveat while the project is pre-1.0: the extracted
**JSON output** is part of the contract as much as the Go API is, so a change to either can
break you. Breaking changes are called out at the top of each release.

Consumers should re-run extraction after upgrading — most releases change the output.

## [Unreleased]

## [0.1.3] — 2026-07-14

The world map becomes a real zoomable tile pyramid, world nodes gain their full graph
(positions, links, CP costs, managers, worker products), NPCs gain client role flags, and item
acquisition resolves to entity refs instead of loose strings. Icons are now WebP.

### ⚠ Breaking

- **`model.Region` is gone, and `data/regions.json` is no longer written.** Region bounds and
  spawns moved onto `WorldRegion` in `world.json`, as `Bounds *Bounds` (`bounds`) and
  `Spawns []Spawn` (`spawns`). Anything reading `regions.json` must move to `world.json`.
- **Icons are WebP, not PNG.** `icons/`, `knowledge_icons/`, `icons/territories/` and the
  zone-category icons. `KnowledgeEntry.image` and `Territory.iconLarge` / `iconSmall` end in
  `.webp`, and id-redirect paths are `icons/<id>.webp`. Region masks (`regionmaps/`) stay PNG.
- **The world map is a packed tile pyramid, not loose PNGs.** `pipeline.WorldMap()` no longer
  writes a flat `worldmap/<layer>/<x>_<y>.png` grid; it writes
  `worldmap/<layer>/tiles.pack` + `meta.json`. Reading it needs a pack reader.
- **Item acquisition changed type.** `Item.Vendors`: `[]string` → `*models.EntityRefList[NPC]`,
  and `Item.GatherNodes`: `[]string` → `*models.EntityRefList[WorldNode]`. The JSON `vendors`
  and `gatherNodes` are now refs, not names.
- **Territory and region links became refs.** `WorldRegion.Territory` and `WorldNode.Territory`:
  `int` → `*models.EntityRef[Territory]`. `NPCSpawn.Region`: `uint32` →
  `*models.EntityRef[WorldRegion]`, with the raw key moved to a new `RegionKey` (`regionKey`).
- **`WorldNode.Kind`**: `int` → `model.WorldNodeKind` (numeric value preserved in JSON).
- **`pipeline.Configure` gained a `region` parameter**:
  `Configure(gameDir, dataDir, lang, region string, pretty bool)`.
- **`pipeline.IconsFresh` removed** — superseded by per-asset provenance tracking.
  `NeedsExtraction` is unchanged.
- **`pipeline.Maps()` no longer produces region masks** — it now runs only the world map.
  `pipeline.RegionMaps()` still does.
- `Store[T].GetAll`, `ResolveUrns` and `ResolveUrnsIn` **compact**: URNs that don't resolve are
  dropped, so the result is not positionally parallel to the input. Use the new `GetAllInto`
  when you need alignment.
- Internal, for anyone vendoring: `tables.DecodeRegions` returns `map[uint32][]model.Spawn`
  instead of `[]model.Region`; `bss.ParseU16OffsetIndex` gained a leading `name string`
  parameter.
- `IconCodecVersion` 1 → 2, new `WorldMapCodecVersion` 1 — both force one full rebuild of the
  assets they cover on the first run of this version.

### Added

- **World-map tile pyramid** — slippy-style `z/x/y` WebP tiles with ocean fill, packed into one
  `tiles.pack` per layer (`BDOTILE1`, documented in `pipeline/tilepack.go`), plus a `meta.json`
  carrying `tilePx`, `unitsPerPixel`, zoom range, grid extent and `oceanColor`. The world layer
  is cropped to the playable region; `morningland` is its own layer.
- **World-node graph** — `WorldNode.Position` and `Links` now come from
  `waypoint_binary/mapdata_realexplore2.bwp`: real in-game map positions and 2,408 directed
  edges. The old exploration.bss anchor is kept as `ExplorationPosition` where it differs. New
  `Children` (non-main neighbours of a main node).
- **Node managers** — `WorldNode.Manager` (owning NPC template) and `ManagerNode` (affiliated
  node → owner), resolved via `characterfunction.dbss`: 493 owners, 417 affiliates. New
  `TownRepresentative` (town ruler NPC).
- **Worker production** — `WorldNode.Products` (`*EntityRefList[Item]`), joined
  `plantzone.dbss` → `plantexchangegroup.bss` → `itemsubgroup.dbss`; 389 of 425 plant zones
  resolve. Quantities and lucky bonuses are server-side and are not inferred.
- **More decoded node fields** — `Main`, `Contribution` (CP cost), `Radius`, `Knowledge`,
  `Special`, `ZoneIndex`, `ZoneCategory`, `GrindZone`, `GrindTier`, `NodeIndex`, `AreaID`,
  `LinkedKey`, `SubKey`, `SubKey2`, `GroupHash`, `Flag`.
- **NPC role flags** — a new `characterspawntype.dbss` decoder fills `NPC.SpawnTypes`: node
  managers (`Explorer`), repairers, stable keepers, wharf managers, market directors and more.
  Role-bearing NPCs that `npcsimply` omits but loc knows about are synthesized into `npcs.json`.
  New `NPCSpawnType` / `NPCSpawnTypes` (46 consts) with `IsMapRole` / `Has` / `HasMapRole`, and
  `NPC.HasSpawnType` / `NPC.HasMapRole`.
- **Service-region support** — new `--region` flag (e.g. `--region na`) and `Options.Region`.
  Region spawn data layers common → `regionclientdata_<lang>_.xml` →
  `regionclientdata_<region>_.xml`, so NPC and monster placements match the region you play on.
  New `pipeline.AvailableRegions(gameDir)` lists what an install ships; `manifest.json` records
  the `region` used.
- **NPC titles are localized** from loc table 6 (e.g. `<Fruit Merchant>`) into `NPC.Title`.
- **Unresolved acquisition text is preserved** — `Item.UnresolvedVendors` and
  `Item.UnresolvedGatherNodes` keep the `<shop>` / `<node region="…">` names that don't resolve
  to an entity, so nothing silently vanishes from an item's acquisition.
- **Zone cross-links** — `zones.json` refs now carry `urn`: ecology → character, topography →
  world region, `node.urn` → world-map node (99 of 105 zones).
- New `WorldNodeKind` (16 consts) and ref constructors `TerritoryRef`, `NPCRef`,
  `WorldRegionRef`, `WorldNodeRef`, `WorldNodeRefList`, `KnowledgeEntryRefList`.
- New `Store[T].GetAllInto(urns, out)` (positional, allocation-free resolution into a
  caller-owned slice) and `StoreFor[T]()` / `StoreForIn[T](r)` (fetch a store once instead of
  paying a registry lookup per URN).
- `EntityRefList[T].AllBulk()` and `.GetManyByIndex([]int)` — bulk, memoized resolution paying
  one registry lookup per call rather than one per element.
- New `utils.IconExt` (`".webp"`) and `utils.IconFileName(ddsPath)`.
- New CLI command `index` (dumps `paz_files.json` / `paz_dirs.json`) and `pipeline.Index()`.
- `NPCSpawn.DialogIndex`; `Zone`'s `Ref` gains `URN`, `NodeRef` gains `Node`.
- `pipeline.Loc()` additionally writes per-table `locs/<table>.json`.

### Fixed

- Region spawn layering now **replaces** a region wholesale instead of appending, so placements
  that a later layer removes or moves are no longer retained from an earlier one.
- A malformed row in a `*_offset.dbss` index (zero size, out of bounds, overlapping) is logged
  and skipped rather than discarding the whole table — previously one bad row from a game patch
  could silently empty `mentaltheme` or `characterstatic`. An index where *no* row is usable is
  still a hard error.
- `EntityRefList.AllBulk` / `GetManyByIndex` no longer mis-align when a URN doesn't resolve: an
  unresolvable entry stays nil in place instead of shifting later entries onto the wrong entity.
- Still-ICE-encrypted DDS textures are detected by their missing `DDS ` magic rather than
  decode-fail-then-retry — deterministic, with no spurious decode failures.
- `exploration.bss` is decoded as a validated cursor walk over the full 117-byte record with
  strict footer tiling. It warns when a byte range that was always zero starts carrying data (a
  patch added a field), and when an unknown node kind appears.
- The build fails if a node references a product item id missing from `items.json`, or if a
  node-manager template has no placement for the selected language/region; it warns when a
  manager's nearest spawn is more than 128,000 units from its node.

### Changed

- Icons are WebP at quality 50 — roughly 40% smaller than the previous PNGs at the sizes they
  render.
- `npcsimply.bss`, `exploration.bss` and the waypoint table are each decoded **once** and
  memoized on the `Builder`, rather than per stage. Build stages reordered: `world` before
  `npcs`, `territories` folded into `world`.
- World-map build: each native DDS is decoded once, WebP-encoded at the finest level and
  box-downsampled in memory for coarser levels (no write-then-read-back). Tiles stream into one
  `tiles.pack` per layer instead of tens of thousands of files, with ocean fill deduped to a
  single stored blob.
- New dependency `github.com/deepteams/webp v1.2.7`; `github.com/dgravesa/go-parallel` promoted
  from indirect to direct. The README no longer claims the CLI is dependency-free, and the CLI
  usage text now lists every command and the `--region` flag.
- New decoders `internal/tables/{characterspawntype,nodemanagers,plantproducts,waypoints}.go`,
  each with unit tests; new tests for `bss/offset`, `tables/regions`, `build/world`,
  `model/npc_spawn_type`, and benchmarks for the ref-resolution paths.
- `FORMATS.md` documents the exploration.bss record layout, `mapdata_realexplore2.bwp`,
  `characterspawntype.dbss`, the worker-product joins, and the new `world.json` / `npcs.json`
  shapes.

## [0.1.2] — 2026-07-12

### Changed (output changed — re-extract)

- Set-effect sections are split by piece count — "Set Effect (2-piece)", "(4-piece)",
  "(6-piece)", "(8-piece)" — instead of every tier collapsing into one "Set Effect".
- ~933 more items get icons: name-only items with no stat record now backfill their icon from
  the archive's id-named icons.
- World-map tiles get clean names: `worldmap/<layer>/<x>_<y>.png` (world / pack / morningland)
  instead of long sanitized paths.
- Item and knowledge icon aliases merged into one `asset_redirects.json` at the data-dir root
  (was `icons/redirects.json`).

### Added

- Icons re-decode only when they need to — an app-only update reuses existing icons instead of
  re-decoding tens of thousands of textures; a game patch or an icon-code bump forces a rebuild
  (game fingerprint + `IconCodecVersion`).
- Unified image-extraction pipeline — item, knowledge, territory, zone-category, region-map and
  world-map images all run through one interface in a single pass over the archive index.

### Fixed

- Territory icons: Valencia and The Great Ocean no longer share (and overwrite) one mark
  texture.
- An output-path bug that wrote files a directory above the intended output.

### Performance

- Parallel DDS/JSON work via `go-parallel` — lower peak memory and faster builds. The
  image-pipeline refactor was verified byte-identical against the previous output.

## [0.1.1] — 2026-07-11

### Added

- **Extraction manifest & staleness API.** `RunAll` writes `manifest.json` into the data dir
  (`{gameFingerprint, appVersion, lang, extractedAt}`). New `GameFingerprint(gameDir)` hashes the
  PAZ index (`pad00000.meta`) plus `ads_version`, and `NeedsExtraction(dataDir, gameDir, appVersion)`
  reports when cached output is stale (game patched, or the embedding app updated). An unreadable
  game dir keeps existing data rather than forcing a rebuild.
- `Options` gains `AppVersion`.

### Changed (output changed — re-extract)

- Item grades: grade 4 relabelled `orange` → `red`, and grade 5 `purple` added.
- Variant grouping now keys on the equip **slot** too, so an appearance/costume item that reuses
  a real piece's name and icon is no longer merged into that combat piece.
- `RunAll` runs build + icons + knowledge-icons only; the loc dump and region-map steps aren't
  consumed by current embedders.

### Performance

- `RunAll` releases the `Builder` after the build step and returns the heap to the OS
  (`FreeOSMemory`) on the way out, so a long-running embedder no longer retains the extraction's
  hundreds of MB (item/enhancement maps, loc tables, the PAZ index).

## [0.1.0] — 2026-07-11

Initial release. Read-only Go CLI and library that decodes Black Desert Online's client data —
the PAZ archives, the `.bss`/`.dbss` binary tables, the per-item recipe XMLs and the `.loc`
localization — into JSON (`items.json`, `recipes.json`, and more), plus decoded item icons.
Distributed via `go install …@latest`.

[Unreleased]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.3...HEAD
[0.1.3]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/iDevelopThings/bdo-data-extractor/releases/tag/v0.1.0
