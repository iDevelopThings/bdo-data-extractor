# Changelog

All notable changes to `bdo-data-extractor` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions are
[semantic](https://semver.org/), with one caveat while the project is pre-1.0: the extracted
**JSON output** is part of the contract as much as the Go API is, so a change to either can
break you. Breaking changes are called out at the top of each release.

Consumers should re-run extraction after upgrading — most releases change the output.

## [Unreleased]

## [0.1.5] — 2026-07-19

Adds five new datasets — adventure journals, quests, class skills, crystal transfusion rules and
lightstone combinations — plus NPC item-services and contribution-point item rentals joined onto
the existing files, and much more complete consumable/buff effect data. The build now writes its
output as an atomic, crash-recoverable transaction — which also makes a full extraction roughly
2.5× faster (~5s, down from ~14s). One JSON field changed shape: `itemType` is now numeric.

### ⚠ Breaking

- **`Item.itemType` is now a number, not a string, and is always emitted.** Was
  `"itemType":"Skill"` (omitempty); now `"itemType":2`. The Go field type changed `string` →
  `model.ItemType` (`uint8`). This is the only wire-breaking change — the large `stat_id_enum.go`
  diff is gofmt whitespace only; no existing `StatId` value changed.

### Added

- **Adventure journals** — the Adventure Log tree (`adventure_journals.json`): journal groups →
  books → ordered quest pages, each page's quest, and every permanent **Family-stat reward** with
  running totals. Quests referenced across the data now resolve to a new `quests.json` carrying
  each quest's accept/complete condition DSL verbatim.
- **Class skills** (`class_skills.json`) — per-class combat and awakening skill-tree grids, each
  rank's identity and active/passive kind, and the decoded stat effects of passive ranks.
- **Crystal transfusion rules** (`crystal_rules.json`) — every transfusion group with its
  max-equipped count, plus the special preset-slot restrictions. Each item's `crystalGroup` now
  also carries its numeric `key`.
- **Lightstone combinations** (`lightstone_combinations.json`) — artifact/lightstone set bonuses
  (name, description, required items, decoded effects) and the amplified-lightstone item-
  equivalence alias map.
- **NPC item-services** on `npcs.json` — shops, secret shops, exchanges and contracts a character
  offers (service name + client access-condition DSL); service-only NPCs are now included too.
- **Contribution-point item rentals** on `items.json` — `buyItemByPoint` offers (item, count,
  point type and cost, condition DSL, dialogue text).
- **Fuller consumable/buff effect data** — resolved effects now carry a canonical `statId` (and
  `statIds`), the buff's module/group, its **stacking category** (food / elixir / perfume / Cron
  meal / …), duration, and whether it's instant; an effect set records which categories it clears
  (e.g. a draught reset). This is the data a stat calculator needs to apply real in-game stacking.
- **New exported enums**: `ItemType`, `FamilyStatType`, `SkillKind`, `CrystalSpecialSlot`; new
  `StatId`s (fear/heat/cold resistance, energy, beast AP, worker stamina, and more); and
  `LifeSkillType.ExpStat`. New domain types for each dataset above; new `EntityRefList` helpers
  (`Contains`/`AddUnique`/`Remove`/`IndexOf`) and a `lightstone-combination` URN domain.

### Fixed

- **Cron-meal and composite-consumable effects were under-reported.** v0.1.4 read only one of an
  item's two skill slots, so consumables that spread their component buffs across the second slot
  (Cron meals, some composite meals) lost part of their effect list. Both slots are now read and
  merged, recovering the complete set.

### Changed

- **Build output is transactional.** Stages decode, validate and register their artifacts first;
  the dataset is published only after every stage succeeds. A failed or interrupted extraction no
  longer partially overwrites the last good output — the next run rolls it back (or completes it
  if it had already committed), and files the build doesn't own are left untouched. The output
  directory gains a `.build-outputs.json` ownership manifest.
- **Buff data is decoded more thoroughly** — records are read through `buffoffset.dbss` with a
  full field decode, buff modules resolve to canonical `StatId`s instead of display strings, and
  several previously-unmapped modules are now decoded (worker-stamina and energy recovery,
  Breath/Strength/Health fitness EXP, death-penalty resistance).
- **Item records are decoded to @212** (a second skill-key slot), and `itemType` is read as the
  new numeric enum. Localization decoding gained three tables (skills, adventure journals,
  lightstone sets).
- The default JSON encoder is now `goccy/go-json` (was the standard library); the encoder sits
  behind an interface with standard and jettison alternatives available.
- The TypeScript enum generator now emits an `<Enum>Names` type and an `as const` `ByName` map.
- `FORMATS.md` and `README.md` document all of the above (the five new datasets, the buff
  offset-index rework, NPC item-services, rentals, and the item-type tooltip labels).

### Performance

- **A full extraction now finishes in ~5s, down from ~14s.** Stages no longer each write their
  JSON as they complete; all writing is deferred to the transactional publish at the end, where
  the artifacts are encoded and written by a bounded pool of workers in parallel (with the two
  bulk arrays, `items.json` and `item_enhancements.json`, written on their own). The `goccy/go-json`
  encoder swap contributes as well.

## [0.1.4] — 2026-07-17

Adds item sets, playable classes with level/fitness progression, and life-skill progression,
plus a symbolic UI-string table. A large internal shift moves the game's enum-like fields —
grades, equip slots, classes, life skills, stats, effect functions — to typed enums generated
from YAML: a single documented source of truth, with compile-time type safety, shared with the
frontend as generated TypeScript. Many decoders were rewritten to consume and validate every
byte of a record, and now fail loudly on an unexpected layout instead of silently dropping rows.

### ⚠ Breaking

- **`Item.grade` is now a number, not a string, and is always emitted.** Was
  `"grade":"white"` (omitempty); now `"grade":0`. Mapping: white 0, green 1, blue 2, yellow 3,
  red 4, purple 5, none -1. The Go field type changed `string` → `model.ItemGrade` (`int8`).
- **`EquipInfo.slot` / `slots` are now numbers, not strings.** `"slot":"Main Weapon"` → `"slot":0`;
  `slots` is now an array of numbers. Types changed `string`/`[]string` →
  `model.SlotName`/`[]model.SlotName`. `slot` also lost `omitempty`, so Main Weapon (0) now
  emits (it was silently dropped in 0.1.3 — see Fixed).
- **`WorldNode.flag` removed**, replaced by `enabled` (bool, always emitted) and
  `unknown17` (bool, omitempty). Anything reading `worldNode.flag` breaks.
- **`src/model/dsl.go` exported tables removed:** `EffectFuncs`, `EffectNamedFuncs`,
  `EffectSectionMarker`. They are superseded by the generated `EffectFuncStat` enum. The JSON
  `effect.func` value is unchanged (still a string). New `EffectFuncToStatMods` can now emit
  several `StatMod`s from one DSL func, so effect arrays may be longer, and `StatMod.note`
  wording changed (don't match on it).
- **`itemenchant`/`enchantstaticstatus` `unknown*` keys renumbered.** The item header/icon
  layout was corrected (~+12 byte shift), so many `unknownNNN` keys in `items.json` and inside
  `enhancement.levels[]` were added, removed, or repointed. These are debug/deviation fields
  with low consumer impact, but the keys changed.
- **`paz_dirs.json` shape changed** (from the `index` command): was
  `{interesting_dirs, dirs}`, now a flat sorted `[]string` of folder names.
- Numeric enum fields serialize as **bare integers** (no `MarshalJSON`): `grade`, `slot`,
  `slots`, and the new class/life-skill enums. `effect.func` and stat ids remain strings.

### Added

- **Item sets** — new `item_sets.json` (`skillpiece.dbss`), each with its N-piece bonus tiers
  and localized bonus text. Every item gains an `itemSets` ref array, and each set lists its
  member `items`. Membership is derived from effect-DSL markers/functions and an explicit
  boss-gear list (Blackstar, Deboreka, Tungrad, Loggia, Geranoa, Manos/Preonne, boss armor, …).
- **Playable classes + progression** — new `character_progression.json`: the real playable
  classes (name, gender, starter/preview weapons), per-class level rules (`experience.bss`,
  incl. the level-60 AP and level-56 DP bonuses), and character fitness curves
  (Breath/Strength/Health, from `fitnesslevel.dbss`).
- **Life-skill progression** — the game's life-skill XP tables are now processed: the max level
  and full experience-per-level curve for each of the 15 life-skill types (`lifeexp.dbss`),
  exposed as `life_skill_progression.json`.
- **Symbolic UI strings** — new `lua-strings` CLI command → `lua_strings_<lang>.json`, decoding
  `stringtable.bss` (~48k `PAGetString` keys) resolved through loc. Not part of the default
  `build`.
- **New exported enum types** (generated, package `model`): `ItemGrade`, `SlotName`,
  `CharacterClassType`, `LifeSkillType`, `LifeSkillGrade`, `EffectFuncStat` (313 DSL funcs),
  `StatId` (158 canonical stat ids). Each carries typed metadata and `FromWire`/`Parse`/`Info`
  helpers.
- **New domain types**: `ItemSet`/`ItemSetBonus`, `CharacterClass`/`CharacterProgression`,
  `LifeSkillProgression`, and richer enchant detail on `EnchantLevel` (`CombatStats`,
  `SpeciesAP`, `EnhancementAids`, `CaphrasMinLevel`/`MaxLevel`). New `urn.ItemSet` domain and
  `ItemSetRefList`.
- **`index` command filters**: `-ignore-exts`, `-only-exts`, `-only-dirs`.
- **A YAML-driven enum generator** (`cmd/enum_codegen`, `go generate ./src/model`) — a large
  shift in how these values are maintained. The enum-like fields above are now defined once in
  `src/model/enums/*.yml`, self-documenting with per-value metadata, and generated into typed Go
  enums (compile-time safety, replacing the old stringly-typed lookup maps) plus matching
  TypeScript, so the backend and the viewer's frontend share one source of truth instead of
  hand-maintaining parallel lists.

### Fixed

- **Caphras `MaxMP` values were wrong** — the last Caphras stat column was decoded as a
  floating-point value in 0.1.3 but is actually an integer, so per-step MaxMP bonuses came out
  garbled. Now read correctly.
- **More buff-stat translations** — buff stats that previously came through untranslated (mount
  HP, horse recovery, stamina/contribution/health EXP, death-penalty resistance, karma, …) now
  carry English labels.

### Changed

- **Decoders now fail loudly on an unexpected layout.** The item, enchant-curve, caphras,
  region-info and exploration decoders (and the caphras build stage) moved from silently
  skipping a bad row / returning a nil no-op to returning an error that aborts the whole build.
  This is safer against shipping a partial or empty table, but a game patch that changes a table
  layout will now block extraction until the decoder is updated, rather than dropping a row.
- **The item decoder was partially rewritten** for stability and to capture more fields — the
  fixed header now extends to @208 with the post-icon block correctly mapped, extra typed fields
  are surfaced, and a malformed record is rejected rather than mis-read.
- **Enhancement (enchant-curve) decoding was largely rewritten**, now fully decoding each curve's
  per-lane melee/ranged/magic combat stats, its species-AP table, and a validated footer, rather
  than reading a partial fixed skeleton.
- **Caphras enhancement decoding was rewritten** with strict validation — totals must increase
  monotonically, stat columns are re-typed to match the game, and NaN/negative stats are rejected
  (see Fixed for the MaxMP correction).
- `regioninfo` was rewritten as a full sequential decode that consumes and validates every byte,
  surfacing many previously-unknown region fields.
- More generally, these decoders now consume and validate every byte of a record instead of
  reading a fixed skeleton and trusting the rest.
- UTF-16 decoding now handles surrogate pairs correctly (lone surrogates → U+FFFD).
- Unknown enum bytes now surface as `UnknownN` instead of an empty string.
- New offset-index parser `ParseU8OneBasedOffsetIndex`, and cursor helpers (`AllZero`, `Zero`,
  `Repeated`) used to consume and assert reserved spans.
- New dependencies: `gopkg.in/yaml.v3` (enum specs), `golang.org/x/tools` (codegen struct
  columns), `github.com/klauspost/compress` (loc zlib), `github.com/goccy/go-json`.
- `FORMATS.md` gained large new sections (item sets, classes, fitness, life skills, the string
  table, and the rewritten `itemenchant`/`enchantstaticstatus`/`regioninfo` layouts); `README.md`
  documents the `lua-strings` command and new outputs.

### Performance

- Faster localization decode at startup — the loc tables now decompress via
  `klauspost/compress` and pre-size their buffers, reducing allocations and GC pressure on boot.
- Item decoding no longer holds onto the source archive buffer: each record's preserved
  variable-length tail is copied into one compact arena, so decoded items don't pin the whole
  decompressed table in memory.
- Safer file reads — decompressed bodies are read to their declared size with explicit
  EOF/overrun checks instead of an unbounded read-all.

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

- **PAZ archives are opened by the name the game actually uses.** They ship uppercase
  (`PAD00001.PAZ`) while the meta beside them is lowercase (`pad00000.meta`); we opened the
  archives as `pad%05d.paz`, which only ever resolved because Windows ignores case. On a
  case-sensitive filesystem every archive open failed, so nothing extracted — reported on Linux
  by [@OniHil](https://github.com/OniHil), who confirmed the casing fix resolves it
  ([#2](https://github.com/iDevelopThings/bdo-data-extractor/pull/2)). Linux is still not a
  tested target: it cross-compiles and the paths are now correct, but no full extraction has
  been run there by the maintainer.
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

[Unreleased]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/iDevelopThings/bdo-data-extractor/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/iDevelopThings/bdo-data-extractor/releases/tag/v0.1.0
