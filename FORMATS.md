# Black Desert Online client data formats

Reverse-engineered notes on the BDO client data files this tool reads. There was no
public documentation for most of it, so it's written down here for anyone who wants to
read their own install's files. Everything below describes file *structure*; no game
data is reproduced.

The client is patched often, so **anchor on structure and position, not on exact
values** — a value scraped from a website is usually a patch or two behind your client,
and your client is the source of truth. Where a scale or offset was found by
correlation (e.g. a constant ratio), that's noted.

Contents:

1. [PAZ archive](#1-paz-archive)
2. [.bss / .dbss tables](#2-bss--dbss-tables)
3. [itemenchant.dbss — item table](#3-itemenchantdbss--item-table)
4. [itemmaxlevel.dbss](#4-itemmaxleveldbss)
5. [enchantstaticstatus.dbss — enhancement curves](#5-enchantstaticstatusdbss--enhancement-curves)
6. [cronenchant.bss — Caphras chart](#6-cronenchantbss--caphras-chart)
7. [Consumable effect chain](#7-consumable-effect-chain-itemskillbuff)
8. [.loc localization](#8-loc-localization)
9. [Recipes](#9-recipes-per-item-xmls)
10. [territoryinfo.bss — territories](#10-territoryinfobss--territories)
11. [regioninfo.bss — regions](#11-regioninfobss--regions)
12. [exploration.bss — worldmap nodes](#12-explorationbss--worldmap-nodes)
13. [NPC / monster / knowledge / drops](#13-npc--monster--knowledge--drops)
14. [Gotchas](#14-gotchas)
15. [Unmapped & contributing](#15-unmapped--contributing)

---

## 1. PAZ archive

Game files live in `Paz/` as a set of `pad*.paz` volumes plus a plaintext index,
`pad00000.meta`.

### `pad00000.meta` (index)

```
[u32 version]
[u32 pazCount]
[pazCount × 12B volume table]        (skipped — not needed to read files)
[u32 fileCount]
[fileCount × 28B PazFile]
[u32 folderNamesLen][ICE-encrypted folder-name table]
[u32 fileNamesLen][ICE-encrypted file-name table]
```

`PazFile` (28 bytes, seven little-endian u32): `hash, folderId, fileId, pazNumber,
offset, compSize, origSize`. A file's path = `folderNames[folderId] + "/" +
fileNames[fileId]`. After ICE-decrypt, the folder table is repeating
`[8B header][NUL-terminated name]`; the file table is repeating `[NUL-terminated name]`.

### Reading a file's bytes

Read `compSize` bytes at `offset` from `pad{pazNumber:05}.paz`, then:

1. If `compSize == origSize` → **stored / plaintext**, use as-is.
2. Otherwise **ICE-decrypt** — *unless* `len % 8 != 0` or the data already begins with
   `PABR` (those are stored unencrypted despite `compSize != origSize`).
3. If the result is an LZ container — `len > 9`, first byte `0x6E`/`0x6F`, **and**
   `u32(data[5:9]) == origSize` — **BDO-LZ-decompress** it. Otherwise truncate to
   `origSize`.

### ICE cipher

Thin-ICE, level 0, 8 rounds, key `51 F3 0F 11 04 24 6A 00`; operates on whole
64-bit big-endian blocks, trailing `len % 8` bytes untouched. See `internal/paz/ice.go`.
BDO-LZ is a custom LZ variant — see `internal/paz/lz.go`.

Many `.dbss` tables are stored plaintext (no ICE, no LZ). If `compSize == origSize`,
the bytes are already readable.

---

## 2. .bss / .dbss tables

Structured data tables. Both physical shapes are an ordered list of records, each an
ordered list of typed fields (`Byte/Int16/UInt16/Int32/UInt32/Int64/Float/Bytes` +
strings). See `internal/bss/reader.go`.

- **PABR** (magic `PABR` = `0x52424150` at offset 0): the **string table is at the
  end**. The last 8 bytes are an `int64` pointer to it; at that pointer sits the string
  table (`[int32 count]` then per-string `[int32 len][bytes][sep]`), immediately
  followed by `int32 rowCount` and the records. String fields are an `int32` index into
  the table.
- **Non-PABR** (first `int32` is the rowCount): records carry **inline** strings — an
  `int64` length prefix then `len × factor` bytes: `UtfText` factor 1 (UTF-8), `Text`
  factor 2 (UTF-16), `UniText` factor 4 (decoded as UTF-16 units). `itemenchant.dbss`
  and `buff.dbss` are this shape.

A schema (`internal/schema`) is an ordered `[]Field`; `ReadAll` walks records by it.
That works for tables with **one uniform record layout** (e.g. `buff.dbss`); tables
with type-conditional layouts (e.g. `itemenchant.dbss`) are read positionally.

### Offset index — `*offset.dbss`

Most data tables have a sibling `<name>offset.dbss` locating each record
`[offset, offset+size)` in the paired `<name>.dbss`. Two index-row layouts are used:

| Row size | @ | Type | Field | Notes |
|---:|---:|---|---|---|
| 12 | 0/4/8 | u32 ×3 | key / offset / size | Column order varies and is detected from valid non-overlapping slices |
| 10 | 0 | u16 | key | Compact form used by character-oriented tables |
| 10 | 2 | u32 | offset | Byte offset in the paired data file |
| 10 | 6 | u32 | size | Record byte length |

The header is either `[u32 count]` or `["PABR"][u32 count]`. For 12-byte rows, the
index may be sorted by key, so offsets are not necessarily monotonic. Detect the
offset/size columns by content: keep pairings whose slices all fit and never overlap,
then break ties by the tightest tiling. See `internal/bss/offset.go`.

---

## 3. `itemenchant.dbss` — item table

The master item table, indexed by `itemenchantoffset.dbss` (key = **full public item
id**). Non-PABR. The index holds both real item rows (key < 10,000,000) and internal
enchant-entry rows (keys ~3e8); a true item row also has `u32 @0 == key`.

### Fixed scalar header

Read positionally from @0 to @196 (`internal/tables/items.go`). Offsets are byte-exact
and unaligned:

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u32 | id | == index key |
| 4 | byte | itemType | `EItemType` |
| 5 | byte | category | `EItemClassify`: MainWeapon/Armor/Accessory/Cook/… |
| 6 | byte | grade | white/green/blue/yellow/orange |
| 7 | byte | equipType | equip slot sub-type → `equipInfo.type` |
| 14 | byte | equipSlot | `__eEquipSlotNo` → `equipInfo.slot` (equip only) |
| 15 | byte | equipKind | 0 Weapon / 1 Armor / 2 Other |
| 16 | byte×46 | extraSlots | front-packed slot list (46 = none); ≤3 used (multi-slot costumes) |
| 62 | byte | itemMaterial | material/model family |
| 63 | i32 | weight | ÷10000 = LT |
| 67 | bool | stackable | |
| 68 | bool | applyDirectly | consumed on obtain |
| 69 | u32 | expirationMinutes | |
| 73 | byte | vestedType | binding enum |
| 74 | bool | userVested | family/character-bound |
| 75 | bool | forTrade | trade-goods item |
| 76 | byte | tradeType | |
| 77 | u64 | classMask | one bit per class; newer classes in the high dword (read u64); all-class = most bits, so test by popcount not equality |
| 97 | byte | requiredLevel | kept when 1 < v ≤ 100 |
| 101 | u32 | maxStack | `0x7FFFFFFF` / `0xFFFFFF00` = unlimited |
| 105 | byte | lifeExpType | |
| 110 | i64 | buyPrice | |
| 118 | i64 | sellPrice | |
| 126 | i32 | repairPrice | |
| 134 | byte | eventType | 0/171 = none |
| 136 | u32 | eventParam1 | |
| 140 | u32 | eventParam2 | |
| 151 | byte | hideFromNote | `shownInNote = (== 0)` |
| 152 | bool | cash | pearl-shop item |
| 153 | byte | cronEnchant | |
| 156 | bool | dyeable | |
| 160 | byte | dyeParts | kept when 0 < v ≤ 30 |
| 184 | bool | personalTrade | |
| 185 | u16 | maxDurability | equipment only; large value = no-durability sentinel |
| 188 | byte | marketCategory id | → localization table 44 |
| 189 | byte | marketSubCategory id | |
| 190 | bool | nodeFreeTrade | |
| 192 | u32 | skillKey | consumables → skill chain (§7) |

Bytes not yet identified are captured verbatim as deviation-only `ItemUnknowns`
(`unknown8`, `unknown85`, …), so the header is fully consumed.

### Name / Icon / EnchantKey (positional)

The pre-name region is type-conditional, so these are located by the icon marker, not
by field walk:

- **Icon** — find the ASCII `New_Icon…​.dds` substring. Its `int64` length prefix is at
  `ic − 8`; the icon string ends at `icEnd = ic + len`.
- **Name** — an inline length-prefixed UTF-16 string ending at `ic − 8`; scan backward
  for the length prefix.
- **EnchantKey** — the `u32` immediately before the Name's length prefix. This is the
  `baseId` linking to the enhancement curve (§5).

### After the icon

- **Post-icon block** (~59 bytes from `icEnd`): among it, `marketable` (bool +0),
  `familyInventory` (bool +13, nonzero = allowed), `bindType` (u8 +15), and
  `marketRegisterLimit` (i64 +42, kept when 0 < v < 2³²).
- **Contribution-point cost** — search from `icEnd` for the 7-byte marker
  `13 06 00 00 00 00 13`; cost = `u32(marker + 20)`, kept when 1..1000 ("[CP]" rental
  gear and placeables).
- **Footer** — ends `[u32 self-id][u16 crystalGroup][0x0100]`; group `!= 0xFFFF` is the
  crystal transfusion group (name + max count from localization table 121).

The ~700–1300-byte binary tail after the icon (per-slot stat arrays, price/tax rates,
scripts, embedded description) is otherwise unmapped (§15).

---

## 4. `itemmaxlevel.dbss`

Via `itemmaxleveloffset.dbss` (key = item id). Each record is `[u32 id][u8 maxLevel]`.
The index has a zero size-column (fixed-stride records), so it's read directly.

---

## 5. `enchantstaticstatus.dbss` — enhancement curves

Per-(item-family, level) stat curve. Record key = `(enhanceLevel << 24) | baseId`, so
`baseId = key & 0xFFFFFF` and `level = key >> 24` (levels 0–25).

Each record is one front-to-back sequential field stream (no fixed offsets), read
largest-type-first — mostly `u32`, with a `u16` block at @53–60 (where a `u32` would
straddle two fields) and lone shift bytes at @24/@59. The meaningful fields:

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u32 | baseId | == key |
| 41 | u32 | enhanceChance | value ÷ 1e6 = base success probability at this level (0 failstacks): 1.0 for +1–7, then falling. Also flags the scheme (below) |
| 53 | u16 | durability | base 100 → PRI 120 / DUO 140 / TRI 160 / TET 180 / PEN 200 |
| 62 | f32 | maxHP | 0 unless the DSL carries `HP_UP(n)` |
| 66 | f32×25 | per-species AP | only slot 1 (@70) is ever nonzero |
| 167 | f32 tri-dice ×7 | AP / defense | slots: —, —, minAP, maxAP, displayAP, damageReduction, evasion |
| 263 | i64 + UTF-16 | effect DSL | length-prefixed (below) |

The remaining header scalars (@4/@8/@25/@45/@55/@57/@60 and the tail rates) are
enhancement-process parameters (material/cron cost, rates), captured verbatim as
`EnchantUnknowns`.

**AP is three dice side by side** — melee, ranged, magic. A sword fills only melee, a
staff only magic; hybrids fill two equally — take the max across the three slots. The
display slot is the game's rounded `(min+max)/2`. `dp = evasion + damageReduction` (base
values).

**The display-stat tail** follows the DSL: an accuracy block — 3× `[i64 dice-len][UTF-16
dice, e.g. "1D3" / "1D7+130"][f32 value]` — then a defense block — 3× `[f32 evasion][f32
addedEvasion][f32 damageReduction][f32 addedDamageReduction]`. Accuracy and the `+N`
added-defense values appear **only** here (take the max of the three slots); base
evasion/DR duplicate the header.

**Effect DSL** — a length-prefixed UTF-16 string at @263: a `;`-separated list of
`NAME(args)` formulas (item + set effects) — `HP_UP(110)`, `MON_DAM_REDUCE_ADD(10)`,
`NO_3_SET_EFFECT()`, `ALL_AP_INCRE()`. Parsing notes:

- Func names are usually SCREAMING_CASE but some are mixed-case (`Donkey_Harness_SET_EFFECT_1_2`).
- Args can be fractional (`ALCHEMY_REDUCE_TIME_DOWN(0.7)`) or roman numerals
  (`MERMAID_HOPE_ADD(IV)` = the tier).
- Argless funcs are markers (`ITEM_EFFECT`, `POTENTIAL_EFFECT`), enhancement-scaling
  effects whose value is the item's own stat curve (`ALL_AP_INCRE`, `ALL_HIT_INCRE`),
  set-effect references, or a few **family constants** whose magnitude is baked into the
  client rather than the data — e.g. `NU_/KU_ALL_REG_ADD` = "All Resistance +10%"
  (Nouver / Kutum). The generic `ALL_REG_ADD(n)` carries its value in the arg.

**Enhancement level names.** Levels are named "+1"…"+15" then PRI…DEC, or PRI…DEC
directly (accessories and the post-PEN boss/season lines). The scheme is not derivable
from `maxEnhance` or category alone — a `maxEnhance`-5 curve is `+1…+5` for basic gear
but `PRI…PEN` for an accessory. The reliable signal is level-1 `enhanceChance`: the game
always grants +1 (chance 1.0) but never PRI, so a non-accessory whose level-1 chance is
below 1.0 is a roman-from-1 line (Sovereign, Fallen God). Accessories are always
roman-from-1, including the guaranteed Tuvala lines (chance 1.0):

```
romanFromOne = category == "Accessory" || level1.enhanceChance < 1.0
```

Roman tiers then start at level 1 (roman-from-1) or level 16 (after the +1…+15 phase).
(Some boss lines — Fallen God, Labreska — additionally show named stages, e.g.
"Obliterating", from the enhanced item's localized name rather than the roman tier.)

**Item → curve link.** `EnchantKey` (§3) is the `baseId`; attach the curve when its
level count equals the item's `maxLevel + 1`. Non-enhanceable equipment (no/zero max
level — artifacts, lightstones, old reward gear) still carries its base stats + effect
DSL in a single level-0 curve (e.g. `SHORT_AP_UP(4)`), so a single-level curve attaches
to those Equip rows too.

---

## 6. `cronenchant.bss` — Caphras chart

The complete Caphras cost/stat chart. The system's internal name is **CronEnchant** (UI
lua: `itemSSW:getCronKey()` → `ToClient_GetCronEnchantWrapper(cronKey, enchantLevel,
gradeIndex)`, stats via `getAddedDD/HIT/DV/HDV/PV/HPV/MaxHP`) — which is why searching
for "caphras" finds nothing.

PABR, 10 fixed rows = the 10 equipment categories (cronKey 1..10). Each row is
`u32 groupCount(3)` then 3× `[u32 stepCount(20)][20 × entry]` — the three
Caphras-eligible enhancement levels (18/19/20 = TRI/TET/PEN) × 20 Caphras steps. Each
39-byte entry:

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u8 | cronKey | == the row's key (1..10) |
| 2 | u8 | enchantLevel | 18/19/20 |
| 3 | u32 | totalStones | cumulative Caphras Stones to reach this level |
| 7 | f32×8 | added stats | getter order: DD (AP), HIT (accuracy), DV (evasion), HDV (hidden evasion), PV (DR), HPV (hidden DR), MaxHP, MaxMP |

**The item → category (cronKey) mapping is not a stored field** — it's computed in the
client executable. But it follows the equipment taxonomy exactly, so the build derives
it from client-side fields:

- eligibility: max enhancement level 20, category MainWeapon/SubWeapon/Armor, grade ≥
  green, not a multi-slot life outfit;
- tier by buy price (disjoint bands: boss ≥ 10M, blue ≥ 2M, else green):

|            | boss | blue | green |
|---|---|---|---|
| main hand  | 1 | 3 | 4 |
| awakening  | 2 | 3 | 4 |
| off-hand   | 5 | 6 | 6 |
| armor      | 7 | 9 | 10 |

Cost tiers pair up (rows 1/2 and 4/5 share weapon charts, 7/8 the top armor chart).
`tables.DecodeCaphras` → `caphras.json`; the chart is also embedded per item as
`enhancement.levels[].caphras`. Step stats are emitted as DSL effects in the same
`{func, args}` shape as enhancement effects, with two extension names for the hidden
stats (`HIDDEN_EVA_INCRE`, `HIDDEN_DAM_REDUCE_INCRE`).

Why it's easy to miss: the totals are cumulative u32s spaced 39 bytes apart, the stat
ramps are floats, and the system is named "cron", not "caphras" — so find a system by
its internal name (via the UI lua getters) and scan value sequences at a candidate
record's stride, not contiguously.

---

## 7. Consumable effect chain (item→skill→buff)

Food and elixirs are `itemType = "Skill"`: using one casts a skill that applies buffs.
The data is a three-table chain (`internal/tables/buffs.go`). A consumable's `skillKey`
(`u32 @192` in the item row) indexes `skilloffset.dbss` → a record in `skill.dbss`, which
carries the cooldown (`u32 @95`, ms; kept when >0, ≤1e8, %1000==0) and a `u16` buff-index
list from `@99` (read until a 0, or an index absent from `buff.dbss`). Each index →
a `buff.dbss` record; English effect names come from localization table 5 (key1==0).

### `buff.dbss` schema

Non-PABR, uniform record layout (validated to consume all 44,178 records), read with a
schema (`internal/schema/buff.go`). Fields in order (strings are variable-length, so the
schema is walked sequentially rather than by fixed offset):

| Type | Field | Notes |
|---|---|---|
| u16 | Index | |
| Text | Name | UTF-16 |
| i16 | Category | |
| u8 | CategoryLevel | |
| u8 | Level | |
| i16 | Group | |
| i16 | ConditionType | |
| u8 | ModuleType | selects the effect module (below) |
| u8 | BuffType | |
| u8 | IsAbsolute | |
| u8 | IsOverlapped | |
| [92 bytes] | EffectData | module arguments (below) |
| i32 | DurationMs | |
| [25 bytes] | — | unmapped |
| Text | ApplyToGroup | |
| UtfText | Icon | UTF-8 path |
| byte, i32 | — | unmapped |
| Text | Desc | |
| [27 bytes] | — | unmapped |

### Effect modules (structured effects)

`EffectData` is a fixed 92-byte argument container (every nonzero byte falls inside it):

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u8 ×7 | flags | application flags |
| 7 | {i32 value, i32 aux} ×10 | slots | the module's arguments (aux only ever 0 / -1) |
| 87 | u8 ×5 | tail | more flags |

Each buff applies **one effect module**: `ModuleType` selects the kind, and the module
reads its arguments from the slots by index — like a function call. Percent amounts are
stored ×10000 (+15% = 150000), flat amounts ×1. This is the primary decode
(`internal/tables/buffmodules.go`), no localized text involved. `ModuleType` is
internally `BuffType`; the client leaks the first six values into lua
(`CppEnums.BuffType`: 1–3 = Current/Max/Regen HP, 4–6 = the MP triplet), matching this
decode. The remaining names aren't in any client data table, so those signatures are
reverse-engineered and hand-named. Representative signatures (full table `buffModules`,
~45 modules):

| module | signature | meaning |
|---|---|---|
| 2/3/5/6/8 | (amount) | Max HP / HP Recovery / MP / MP Recovery / Stamina |
| 29 | (amount×10000) | Weight Limit (LT) |
| 9/10/11/30/50/57 | (amount%) | Move/Attack/Cast Speed, Crit Rate, Mount EXP, Drop Rate |
| 25 | (amount%, kind, lifeSkill) | kind 0 Combat / 1 Skill / 2 life-skill EXP |
| 39/40/41/43 | (target, amount) | target 0 melee / 1 ranged / 2 magic / 3 all — AP/Acc/Eva/DR |
| 46 | (species, amount) | extra AP vs Humans/Demihumans/Beasts/… |
| 49/105 | (kind, amount%) | CC resistance / ignore-resistance |
| 67 | (kind, ranks) | potential slots |
| 93 | (kind, amount%) | special-attack extra damage |
| 80 / 149 | (lifeSkill, amount) / (lifeSkill, _, amount) | life-skill EXP / mastery |
| 128 | (kind, amount%) | weather resistance |
| 95 | (ms) | Underwater Breathing (sec) |

Life-skill index (25/80/149): 0 Gathering, 1 Fishing, 2 Hunting, 3 Cooking, 4 Alchemy,
5 Processing, 6 Training, 7 Trading, 8 Farming, 9 Sailing, 11 Barter, 15 all.

A few modules pick their value slot/scale/unit from a parameter, so they have explicit
resolvers (`customModules`): **1/4** HP/MP — value slot 0, `ConditionType` selects the
trigger (recovery on Hit/Crit, or, for negative values, "Fixed Damage on Back Attack /
Critical Hits / Retaliation"); **111** manufacturing — value slot 1, slot 0 selects
Alchemy/Cooking Time or Processing Success Rate; **120** Monster DR — slot 0 = Rate% vs
flat; **136** extra AP — the value's *slot* is the variant (vs Monsters / Adventurers).

Fallbacks: a module not in the table falls back to the localization-table-5 English name parsed into
`{stat, op, value, unit}`; a buff with neither is *hidden* (the game doesn't show it
either) and named from the Korean. A "master" buff (a draught's headline) carries the
full multi-line text in localization table 5 while its component buffs are Korean-only — which is why
text parsing alone under-counts, and the module decode is primary. `BuffType` is not a
usable shown/hidden flag (it's `1` for nearly everything).

---

## 8. `.loc` localization

`ads/languagedata_<lang>.loc` (in the install dir, not the PAZ; read-only). The file is a
`u32 declaredDecompressedSize` followed by a zlib stream of these records:

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u32 | textLen | UTF-16 code units |
| 4 | u32 | key0 | string table / category |
| 8 | u32 | id | |
| 12 | u32 | key1 | field selector (below) |
| 16 | UTF-16LE ×textLen | text | |
| — | u32 | terminator | always 0 |

`key1` is a packed selector: its high byte is the field/column (`0` = name,
`0x01000000` = description, `0x02000000`/`0x03000000` = further columns). The English
file is ~1.38M strings across ~114 `key0` tables. The ones this tool joins:

| key0 | contents |
|---|---|
| 0 | items — name(0), description, use-text, exchange text |
| 1 | titles |
| 5 | buff / effect display names (key1==0) |
| 6 | entity names — classes, creatures, NPCs, resources (NPC English names) |
| 9 | knowledge theme / category names |
| 12 | territories — field 0 = nation, description = territory name |
| 17 | topography — place / region names |
| 18 | quests — name, description, giver, objective |
| 29 | worldmap node names |
| 34 | knowledge card name / description / acquisition |
| 44 | central-market categories (see below) |
| 115/116/117 | Monster Zone Info sub-category / zone / tag names |
| 121 | crystal transfusion group — id = group, key1 = max count, text = name |
| 123 | workshop / house names (by `eHouseIconType`) |

Internal table text (Name fields in `.dbss`) is **Korean** even on the EU client; the
display text is resolved through `.loc` by id. Searching the binaries for English finds
nothing — search Korean (UTF-16).

### Central-market categories — localization table 44

Keyed by the main-category id (item `@188`). Within each entry: `key1 == 0` is the main
name ("Consumables"); `key1` in `1..0xFFFF` are the sub-category names (matching item
`@189`); `key1 ≥ 0x10000` are per-category enhancement-level display labels (skipped).

---

## 9. Recipes (per-item XMLs)

Crafting recipes come from the per-item info XMLs in the PAZ
(`internal/tables/recipexml.go`). Each file is an `<itemInfo>` document for the item it
produces:

| XML path | Field / attribute | Meaning |
|---|---|---|
| `<itemInfo>/<itemKey>` | text u32 | Output item id and file identity |
| `<cook>/<item>` | `<id>`, `<count>` | Cooking ingredient id and quantity |
| `<alchemy>/<item>` | `<id>`, `<count>` | Alchemy ingredient id and quantity |
| `<manufacture>` | `action` | Processing type, e.g. `MANUFACTURE_HEAT` or `MANUFACTURE_GRIND` |
| `<manufacture>/<item>` | `<id>`, `<count>` | Processing ingredient id and quantity |
| `<house>` | `type` | `eHouseIconType`; localization table 123 supplies the workshop/station name |
| `<house>/<item>` | `<id>`, `<count>` | Worker-building ingredient id and quantity |
| `<shop>/<character>` | `<name>` | Vendor NPC name |
| `<collect>/<character>` | `<name>` | Gather/collect source name |
| `<node>` | `region` | Production/gather-node name |

Repeated producing blocks are alternative recipes. `MANUFACTURE_ALCHEMY` and
`MANUFACTURE_COOK` are the Processing-window Simple Alchemy/Cooking actions and are kept
distinct from real `<alchemy>` and `<cook>` recipes. House type examples include 8 =
Jeweler, 9 = Tool Workshop and 18 = Costume Mill.

Raw/gathered-material candidates come from `ui_html/xml/<lang>/itemmaking.xml`:

| XML path | Field / attribute | Meaning |
|---|---|---|
| `<nodeProduct>/<item>` | `key` u32 | Node-product item id |
| `<collect>/<item>` | `key` u32 | Gathered item id |
| `<fishing>/<item>` | `key` u32 | Fishing item id |

Candidates with a real production recipe are removed before the final gathered flag is
set, because this palette also contains some processed items.

---

## 10. `territoryinfo.bss` — territories

The 14 world territories (Balenos, Serendia, Calpheon, …) in game order. PABR, UTF-16
string table. Byte-packed but fully regular — the records tile `[8, stPtr)` exactly
(12×88 + 2×92 bytes):

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u16 | index | sequential 0..13; == localization table 12 id |
| 2 | u8 | primary | 1 = the nation's direct/primary territory |
| 3 | u8 | autonomous | 1 = autonomous (Balenos, Serendia) |
| 4 | f32 vec3 ×3 | markPositions | worldmap territory marks (zeroed = unused) |
| 40 | u32 | nationKey | hash shared by all territories of a nation |
| 44 | u32 | nationStrIdx | Korean nation name |
| 48 | u32 | nameStrIdx | Korean territory name |
| 52 | u32 | iconLargeIdx | worldmap-mark .dds |
| 56 | u32 | iconSmallIdx | worldmap-mark .dds |
| 68 | u32 | crownItemId | territory-conquest crown item (loc-t0 id) |
| 72 | u32 | armorItemId | conquest armor item |
| 76 | u32 | hasExtra | 0/1 |
| 84 | u32 | extraKey | present only when hasExtra == 1 (unidentified) |

The interleaved const-2 / zero fields (@60/@64/@80 and the trailing u32) are validated.
`tables.DecodeTerritories` checks every invariant and the exact tiling, so an incompatible
layout fails loudly. English names join localization table 12 (field 0 = nation,
description = territory). Folds into `world.json`.

---

## 11. `regioninfo.bss` — regions

Every map region (1,572): key, names, **territory membership**, world position, and
warehouse groups. PABR, UTF-16 string table. Records are byte-packed: a fixed 389-byte
skeleton plus two length-prefixed lists, so consecutive records rotate u32 alignment by
one byte (389 ≡ 1 mod 4) — under an aligned scan every field smears across four
byte-rotations. Layout (offsets from record start):

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u16 | regionKey | == localization table 17 id (English names) and the regionclientdata key |
| 6 | u8 | regionType | 1 = town/city, 2 = field, … |
| 32 | u32 | const 19950 | anchor A |
| 90 | u8 | territoryIndex | == `territoryinfo` / localization-table-12 index (Velia → 0 Balenos, …) |
| 92 | u16 | nameStrIdx | own Korean name |
| 96 | u16 | capitalNameIdx | the territory capital's Korean name |
| 100 | u16 | capitalKey | the territory's capital region — constant per territory (Balenos → 5 Velia, Mediah → 202 Altinova) |
| 131 | f32 ×3 | position | world x/y/z |
| 149 | u32 | const 0x13524B01 | anchor B / format-version marker |
| 210 | u32 | warehouseGroupCount | |
| 214 | u16 ×n | warehouseGroup | region keys in this storage/transport group, incl. itself; only the 58 warehouse-bearing places carry it — groups are disjoint, matching the transport topology |

After the group list: `u32 extraPositionCount` + n×vec3f (extra worldmap marks — only
the Great Desert of Valencia), then a fixed color/const tail. Record size =
`389 + 2×warehouseGroupCount + 12×extraPositionCount`.
`tables.DecodeRegionInfo` re-validates both anchors per record and the exact tiling, and
returns the per-territory capital map (validating that `capitalKey` is constant within a
territory). Unidentified: `+2..+5`, `+7`, the float params at `+153`, six ~105k ids at
`+185` (towns only), and the color tail — none needed for the geographic database.

Anchor B behaves as a format-version marker. If it differs, identify the u32 that is
constant at `+149` across all anchor-A records and verify exact record tiling before
accepting the value.

### `regionclientdata*.xml` — placed characters

Each file is a flat stream of region elements. Repeated `RegionInfo` keys within one
file append spawns; an empty region is meaningful because a higher-priority layer can
clear the baseline region's placements.

| Element | Attribute | Type | Meaning |
|---|---|---|---|
| `RegionInfo` | `Key` | u32 | Region key; joins `regioninfo.bss` and localization table 17 |
| `SpawnInfo` | `key` | u32 | Character-template key; joins `npcs.json` |
| `SpawnInfo` | `dialogIndex` | i32 | Placed/dialog variant; external maps often call this `sub_id` |
| `SpawnInfo` | `position` | `{f64,f64,f64}` | World x/y/z placement |

Files apply by whole `RegionInfo Key` in this order:

| Layer | Example | Behavior |
|---:|---|---|
| 1 | `regionclientdata.xml` | Common baseline |
| 2 | `regionclientdata_en_.xml` | Language/resource baseline replaces matching regions |
| 3 | `regionclientdata_na_.xml` | Service-region data replaces matching regions |

This is region replacement, not an individual-spawn union: retaining both versions
would leave thousands of moved or removed placements in the output.

### `region_info.xml` — region bounds

Multiple boxes for the same key are unioned into one AABB.

| Element | Attribute(s) | Type | Meaning |
|---|---|---|---|
| `box` | `region_index` | u32 | Region key |
| `box` | `aabb_min_x/y/z` | f64 ×3 | Minimum world-space corner |
| `box` | `aabb_max_x/y/z` | f64 ×3 | Maximum world-space corner |

Output: **`world.json`** `{territories, regions, nodes}`. English names come from
localization table 12 for territories, table 17 for regions, and table 29 for nodes.
Each region also carries its world-space `bounds` (`region_info.xml`) and `spawns`
(`regionclientdata` NPC/monster placements); `zones.json` contains monster zones. The
game may store one region record per **spawn phase** of a place, such as quest or
day/night states. Records sharing a name and position are variants; the lowest-key
record is canonical and the others reference it through `variantOf`.

---

## 12. `exploration.bss` — worldmap nodes

The 1,037 worldmap nodes (the node-manager network). PABR, UTF-16 string table.
Byte-packed: a fixed 117-byte head, then seven counted u32 lists per record, then a
footer table after the last record:

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u16 | nodeKey | == localization table 29 id and the node ids community sites use |
| 2 | u16 | unknown2 | Usually zero |
| 4 | u8 | flag | Normally 1; retained verbatim |
| 5 | u8 | nodeKind | 16-value `model.WorldNodeKind` enum |
| 6 | u16 | linkedKey | redundant node reference; == `nodeKey` in all 1,037 records |
| 8 | u16 | unknown8 | Usually zero |
| 10 | u16 | nameStrIdx | Korean node name |
| 12 | u8 ×2 | zero | Reserved |
| 14 | u8 | const | 1 |
| 15 | u8 | zero | Reserved |
| 16 | u8 | networkFlag | 1 = normal network node; 0 = special town/sea/district/battlefield location |
| 17 | u8 | mainCopy1 | Copy of the main/sub state at +116 |
| 18 | u8 | mainCopy2 | Copy of the main/sub state at +116 |
| 19 | u8 | zoneIndex | Sparse special-content index |
| 20 | u8 | zoneCategory | 1 island; 2 coastal; 5 inland/desert; 6 battlefield/ocean |
| 21 | u8 | grindZone | Sparse Marni/Elvia grind-zone index |
| 22 | u8 | grindTier | Recommended-AP tier; observed values include 2 and 3 |
| 23 | u32 | subKey | Waypoint-space/internal key |
| 27 | u32 | subKey2 | Copy of `subKey` on 997/1,037 nodes; zero on the others |
| 31 | f32 | radius | Map influence radius |
| 35 | f32 | radiusSquared | Cached `radius²` |
| 39 | u32 | unknown39 | Small internal value; 13 distinct values observed |
| 43 | u16 | managerFamilyCharacterId | character-template id repeated across a managed node family |
| 45 | u16 | representativeId | town ruler/representative character id; joins `npcs.json` |
| 47 | u32 | packedNodeIndex | `0x20000` flag plus low 17-bit node enumeration; build retains the low 17 bits |
| 51 | u32 | packedAreaId | `areaId << 16`; 44 world-map areas/sectors observed |
| 55 | u8 ×36 | zero | Reserved |
| 91 | u8 ×3 | zero | Reserved |
| 94 | u8 | contribution | Contribution-point cost; observed range 0–3 |
| 95 | u8 ×9 | zero | Reserved |
| 104 | f32 ×3 | explorationPosition | exploration/label anchor; not a reliable parent relation |
| 116 | u8 | subFlag | 0 = main node; 1 = sub-node |

`nodeKind` is the client's `CppEnums.ExplorationNodeType`: Normal=0, Village=1,
City=2, Gate=3, Farm=4, Trade=5, Collect=6, Quarry=7, Logging=8, Dangerous=9,
Finance=10, FishTrap=11, MinorFinance=12, MonopolyFarm=13, Craft=14, and
Excavation=15. The table adds the practical meaning observed on the world map:

| Value | Enum | Observed meaning |
|---:|---|---|
| 0 | Normal | Generic field, location, island, or connecting node |
| 1 | Village | Town, village, settlement, or minor hub |
| 2 | City | Major city or capital |
| 3 | Gate | Gateway, outpost, fort, or guard camp |
| 4 | Farm | Crop-production sub-node |
| 5 | Trade | Farm, ranch, or resource-camp main node |
| 6 | Collect | Gathering production |
| 7 | Quarry | Mining production |
| 8 | Logging | Lumber production |
| 9 | Dangerous | Dangerous/combat-site main node |
| 10 | Finance | Town asset-management service node |
| 11 | FishTrap | Fish-drying production |
| 12 | MinorFinance | Worker investment-bank production |
| 13 | MonopolyFarm | Specialty production |
| 14 | Craft | Animal-product or other crafting production |
| 15 | Excavation | Excavation or special-workshop production |

From @117, every record ends with seven counted lists:

| List | Layout | Meaning |
|---:|---|---|
| 0 | `[u32 count][count × u32]` | Coarse regional grouping hash; 33 distinct values |
| 1–5 | `[u32 count][count × u32]` | Knowledge-entry keys associated with the node |
| 6 | `[u32 count=0]` | Empty in every observed record |

After the final node record is a global footer:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | count | 111 entries in the observed dataset |
| 4 + n×6 | u16 | unknownA | Small lookup/index value; meaning unresolved |
| 6 + n×6 | u16 | nodeKey | Always a valid exploration node key |
| 8 + n×6 | u16 | zero | Reserved; always zero |

`tables.DecodeExplorationNodes` validates the list counts and exact footer tiling.
The actual per-node position and connection graph are in
`waypoint_binary/mapdata_realexplore2.bwp`; a node→territory field is not stored, so the
build derives territory from the nearest region by x/z distance. A main node's children
are its directly connected non-main neighbors in that graph; this matches every shared
public plant-zone parent relation, whereas @104 co-location does not.

### `characterfunction.dbss` — manager-family owner ordering

The compact 10-byte offset index is keyed by character-template id. The full variable
record remains only partially mapped; the manager join uses one exact counted list found
inside it:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| v | u32 | nodeCount | Matches the number of nodes carrying this character id at exploration +43 |
| v + 4 | u32 × nodeCount | orderedNodeKeys | Same set as the exploration family; first key is the owner |

In plain terms, exploration +43 identifies the NPC-manager family shared by a main node
and its production nodes. `characterfunction.dbss` independently lists the same family
in order: the first node owns the manager and the remaining nodes refer to that owner.
`world.json` represents these as `manager` and `managerNode`. Exploration +45 is a
different relation: it identifies town rulers or representatives, such as Igor Bartali
and Crucio Domongatt.

As a validation sample, all 494 raw families and all 914 affiliated nodes match between
the two files. Valid families contain a main node, apart from two standalone kind-4 farm
owners. Four non-main kind-0 pseudo nodes in Islin Bartali's family are therefore not
manager relations. This produces 493 owners and 417 affiliates, and every retained
manager has `SpawnType Explorer=12`.

Contribution cost does not imply that a manager relationship exists. Three dormant or
unreleased-looking main records retain a nonzero cost while their raw +43 field is zero
and no `characterfunction` list references them:

| Node key | Display/raw name | Waypoint internal name | CP | Evidence |
|---:|---|---|---:|---|
| 1651 | Duvencrune | `field(dvenkrun_castle)` | 1 | Duplicate placeholder; the live Duvencrune city is key 1649 |
| 1706 | `UnKnown` | `field(black_mountain_range)` | 3 | Unnamed placeholder linked to O'draxxia |
| 2055 | `UnKnown` | `chungsaislnad` | 1 | Unnamed/unreleased-looking island record |

The build therefore does not synthesize managers or enforce `contribution > 0 ⇔
manager`.

### Worker-production item tables

Worker products are joined through three tables.

`plantzone.dbss` uses the standard 12-byte offset index and variable records:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | nodeKey | Equals the offset-index key; joins `exploration.bss` |
| 4 | u8 ×19 | unknown | Retained only as structural spacing |
| 23 | u32 | packedProductionKey | Low u16 is the production key; high word is ignored |
| 27 | u32 | workerSpeciesCount | 0–32 in validated data |
| 31 | u8 × count | workerSpecies | Allowed worker-species bytes; not needed for product identity |

`plantexchangegroup.bss` is PABR with fixed 94-byte records:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u16 | productionKey | Join from `plantzone` |
| 2 | u16 | productionKeyCopy | Must equal +0 |
| 4 | u16 | unknown | Unmapped |
| 6 | u32 | itemSubgroupKey | Normal-output subgroup |
| 10 | u8 ×80 | unknown | Unmapped |
| 90 | u32 | unknownTail | Unmapped |

`itemsubgroup.dbss` uses the compact 10-byte offset index and variable records:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | subgroupKey | Equals the u16 offset-index key |
| 4 | u8 ×10 | unknown | Unmapped header fields |
| 14 | u32 | itemCount | 0–100 in validated data |
| 18 + n×135 | u32 | itemId | Normal worker-production item |
| 22 + n×135 | u8 ×131 | itemData | Unmapped per-item payload; quantities are not identified here |

`world.json` exposes the resolved normal items as each node's `products` references.
The observed client data resolves 389 of 425 plant zones; 36 reference subgroup keys absent
from `itemsubgroupoffset.dbss` and are left without products. Quantities and lucky bonus
drops are server-side data and are intentionally not inferred here.

### `mapdata_realexplore2.bwp` — node positions and links

PABR with 1,058 fixed 23-byte rows, followed by a counted edge list, five zero bytes,
and a UTF-16 internal-name table:

| Row @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | nodeKey | Exploration key; 22 rows are outside the observed 1,037-node table |
| 4 | u32 | rowIndex | Sequential from zero |
| 8 | f32 ×3 | position | World/minimap x/y/z used for the node marker |
| 20 | u8 ×3 | flags | Client waypoint flags; meanings not fully mapped |

The section after the fixed rows is:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | edgeCount | 2,408 directed edges observed |
| 4 + n×8 | u32 | fromKey | Source node key |
| 8 + n×8 | u32 | toKey | Destination node key |
| after edges | u8 ×5 | zero | Reserved delimiter before the string table |
| string table | UTF-16 strings | internalName | One internal waypoint name per fixed row |

These positions match the in-game map and the public Workerman/Bdolytics waypoint
dumps, including distinct positions for production sub-nodes.

---

## 13. NPC / monster / knowledge / drops

Across the `gamecommondata/binary` set: **NPC/monster identity is client-side, but
granular loot/drop/yield data is server-side and not shipped here.**

### `npcsimply.bss` — NPC identity

PABR with fixed 33-byte records and an 8-bit string table. English names and titles come
from localization table 6; the embedded strings are Korean.

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u16 | characterId | NPC character-template key |
| 2 | u8 ×18 | unknown | Unmapped fixed fields |
| 20 | u32 | packedNameRef | String-table index is `value >> 8` |
| 24 | u32 | packedTitleRef | String-table index is `value >> 8` |
| 28 | u8 ×5 | unknownTail | Unmapped |

### `characterspawntype.dbss` — map/service roles

The data starts with `[u32 count]`, followed by 24,008 fixed 48-byte records. Its
companion uses the compact 10-byte offset index.

| Record @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u16 | characterId | Must equal the offset-index key |
| 2 | u8 ×46 | roleFlags | Byte index is `CppEnums.SpawnType`; every byte is 0 or 1 |

The 46 role indices exposed by `model.NPCSpawnType` are:

| Value | Role | Value | Role |
|---:|---|---:|---|
| 0 | Normal | 23 | Alchemy |
| 1 | SkillTrainer | 24 | GuildShop |
| 2 | ItemRepairer | 25 | ItemMarket |
| 3 | ShopMerchant | 26 | TerritorySupply |
| 4 | ImportantNPC | 27 | TerritoryTrade |
| 5 | TradeMerchant | 28 | Smuggle |
| 6 | Warehouse | 29 | Cook |
| 7 | Stable | 30 | PC |
| 8 | Wharf | 31 | Grocery |
| 9 | Transfer | 32 | RandomShop |
| 10 | Intimacy | 33 | SupplyShop |
| 11 | Guild | 34 | RandomShopDay |
| 12 | Explorer | 35 | FishSupplyShop |
| 13 | Inn | 36 | GuildSupplyShop |
| 14 | Auction | 37 | GuildStable |
| 15 | Mating | 38 | GuildWharf |
| 16 | Potion | 39 | PCRoomStable |
| 17 | Weapon | 40 | Instrument |
| 18 | Jewel | 41 | Unknown41 |
| 19 | Furniture | 42 | TrainingVehicleShop |
| 20 | Collect | 43 | AbyssOneEnterPositionGuide |
| 21 | Fish | 44 | ChangeMarniStone |
| 22 | Worker | 45 | ChurchBuff |

`Explorer=12` identifies node managers. Index 41 is present in observed data but omitted from
the shipped Lua enum, so it remains explicitly unknown.

### `characterstatic.dbss` — interaction/model metadata

This is render and interaction metadata, not combat stats. It uses the compact 10-byte
offset index keyed by character-template id. Records are variable because the first two
scripts are length-prefixed UTF-16 strings. In the table, `p` is the first byte after
`script2`:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 ×8 | header | Unmapped record header |
| 8 | u8 | tag | `0x15` in decoded records |
| 9 | i64 + UTF-16 | script1 | `[i64 charCount][charCount × u16]` |
| after script1 | i64 + UTF-16 | script2 | Same encoding |
| p | u8 | unknown | Unmapped delimiter |
| p + 1 | u32 | characterId | Must equal the offset-index key when parsing lands cleanly |
| p + 5 | u32 | npcKind | Semantic entity-kind bitfield; high combat flags remain partly unmapped |
| p + 9 | u32 ×n | configFields | Raw structured fields retained until the model path |
| variable | ASCII | modelPath | Longest printable path containing `/`, e.g. `npc/...` |

`getknowledge(N);` in either script provides the exact character→knowledge-card link.

**Other present client tables:**

- **`collect.dbss`** + **`collectresourcename.dbss`** — gatherable identity and internal
  mesh names; no yields or rates.
- **`encyclopedia.bss`** — the in-game fish/creature encyclopedia (PABR, 300 × 104-byte
  records). Only the following fields are presently mapped; offsets not listed have not
  been established precisely:

  | @ | Type | Field | Meaning |
  |---|---|---|---|
  | 0 | u16 | id | Encyclopedia entry id |
  | unknown | integer | knowledgeCode | Knowledge-card relation |
  | unknown | f32 ×2 | sizeRange | Creature/fish size values |
  | unknown | integer ×2 | descStrIdx, iconStrIdx | Description and artwork `.dds` string references |

`npcs.json` joins `npcsimply` identity, `characterspawntype` roles and
`regionclientdata` placements. Its id is a character-template key; each `spawns` entry is
a placed variant of that template and retains `dialogIndex`, the same variant key
external maps call `sub_id`. All 492 node-manager IDs in the referenced Bdolytics node dump
carry client `SpawnType Explorer=12`; the role does not require external data. The build
requires every emitted manager template to resolve to a placement after spawn layers are
applied. The four non-main kind-0 pseudo nodes in the Islin Bartali family are not
manager relationships and emit neither `manager` nor `managerNode`. The build warns when
a retained manager's nearest placement is more than ten world-map tiles (128,000 units)
from its owner node.

### Knowledge / Ecology → `knowledge.json`

`mentaltheme.dbss` is the 901-node category tree and uses the compact 10-byte offset
index:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u16 | themeKey | Equals the offset-index key |
| 2 | i64 | nameLength | UTF-16 code-unit count |
| 10 | u16 × nameLength | sourceName | Embedded source-language name |
| 10 + 2×nameLength | u16 | parentTheme | Parent category key; zero for a root |

`mentalcard.dbss` contains 12,077 knowledge entries and uses the standard 12-byte offset
index. Its fixed header is:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | cardKey | Equals the offset-index key |
| 4 | u32 | themeKey | Owning knowledge category |
| 8 | f32 | minFavor | Conversation favor parameter |
| 12 | f32 | maxFavor | Conversation favor parameter |
| 16 | f32 | interest | Conversation interest parameter |
| 20 | u32 | flags | Obtain/display flags; not an entity kind |
| 24 | u32 ×4 | packedFields | Partly mapped packed sub-structure; +36 is zero on default cards |
| 40 | variable | embeddedStrings | Source name/description plus ASCII `.dds` image path |

English names come from localization table 9 (themes) and localization table 34 (cards; description +
acquisition columns). **Links are by localized name, not id** — the id spaces overlap
coincidentally. The "You can learn about X" items (`itemType == "Skill"`) match a theme
name (group item) or card name (single item); a card's NPC is matched by name to localization table 6.
`knowledgelearning*.bss` is card↔card (learning prereqs), not the item link.

### Monster-zone obtainable loot — client-side

The curated obtainable loot per monster zone — the in-game "Monster Zone Info" window —
is in **`dropuihuntinggroundinfo.bss`**: alongside 105 zone names it holds the item-id
list per zone (≈748 references, 361 distinct items). The record layout is
nested/hierarchical (a zone-grouping section with waypoint-key hashes, then per-zone item
blocks split into Equipment / Other), so exact per-zone attribution needs the
record-boundary work finished. Companion `dropui*` tables are the filters
(`dropuimaincategoryinfo`, `dropuisubcategoryinfo`, `dropuitaginfo` = tags).

### Server-side / not in the client common-data

Granular **per-item drop rates** (the window shows one zone-level rate, not per item);
**monster combat stats** (HP/AP/DP/level — the window's recommended AP/DP is joined from
elsewhere); **gatherable item yields**; per-recipe **life-skill tier conditions**; and
**bundle/unboxing contents** (`itembundle.bss` is empty).

---

## 14. Gotchas

- **Many `.dbss` are stored plaintext** — don't assume ICE/LZ when `compSize == origSize`.
- **Unaligned offsets** — item prices/weight are at 63/110/118/126; 4-aligned scans
  can't find them.
- **Weight is ÷10000** (not ÷100/÷1000).
- **Internal text is Korean** (UTF-16) — match Korean, not English.
- **Inline strings are length-prefixed** — an `int64` count then the bytes; read by the
  prefix, not by scanning for the end.
- **`itemenchant` is not a flat schema** — the pre-name region is type-conditional;
  reach Name/Icon/EnchantKey positionally (via the icon marker).
- **Offset indices can be sorted by key** — detect the offset/size columns by bounds +
  non-overlap, not by assuming they're cumulative.
- **Byte packing looks like corruption** — several tables (`regioninfo`, `exploration`,
  `territoryinfo`) are byte-packed with a size that isn't a multiple of 4, so under a
  u32-aligned scan the records "smear" and look irregular. Hexdump a whole record region
  at byte granularity before concluding the serialization is irregular.

---

## 15. Unmapped & contributing

This was reverse-engineered from scratch with no reference, so plenty is still open. If
you have schema, headers, field layouts, or any information on the below, please open a
PR or an issue — even partial notes help.

- **`itemenchant` post-icon tail (~185 columns).** ~700–1300 bytes after the icon:
  per-slot stat arrays, detailed price/tax rates, pet-feed slots, trade/bind/expiry
  flags, an embedded description, and scripts. Everything before it is read positionally.
- **`itemenchant` pre-name variant block (gear).** A type-dependent block between the
  fixed header and the name; ~744 set/boss pieces carry a larger `count`+entries block.
  The entry layout and its trigger sub-type are unmapped (Name/Icon/EnchantKey are
  reached positionally past it).
- **`buff.dbss`** — the flag bytes around `EffectData`/`DurationMs` and the ~166
  effect-module names that aren't in any client table.
- **`dropuihuntinggroundinfo.bss`** — the per-zone record boundaries, to attribute each
  item to its exact zone (§13).
- **`itemexchangesource.bss`** (3 MB PABR, 13,407 rows, variable-length) — an item
  "obtained-from" table, partially decoded.
- Per-monster **drop rates** and **monster combat stats** appear to be server-side.
