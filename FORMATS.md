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
   - [skillpiece.dbss — item-set definitions](#skillpiecedbss--item-set-definitions)
   - [lightstoneset.bss — artifact/lightstone combinations](#lightstonesetbss--artifactlightstone-combinations)
6. [cronenchant.bss — Caphras chart](#6-cronenchantbss--caphras-chart)
7. [Consumable effect chain](#7-consumable-effect-chain-itemskillbuff)
   - [Player skill groups and passive stats](#player-skill-groups-and-passive-stats)
8. [.loc localization](#8-loc-localization)
   - [Crystal transfusion rules](#crystal-transfusion-rules)
   - [Adventure journals and family stats](#adventure-journals-and-family-stats)
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

1. If `compSize == origSize` → **stored**, use as-is initially.
2. Otherwise **ICE-decrypt** — *unless* `len % 8 != 0` or the data already begins with
   `PABR` (those are stored unencrypted despite `compSize != origSize`).
3. If the result is an LZ container — `len > 9`, first byte `0x6E`/`0x6F`, **and**
   `u32(data[5:9]) == origSize` — **BDO-LZ-decompress** it. Otherwise truncate to
   `origSize`.
4. A small number of stored binary tables carry one additional ICE layer. When the
   result is not `PABR` and its length is divisible by 8, ICE-decrypt a copy and keep
   it only if that copy begins with `PABR`. `commonlifestatdata.bss`,
   `pcgrowthsimply.bss` and `dropuimaincategoryinfo.bss` use this form.

### ICE cipher

Thin-ICE, level 0, 8 rounds, key `51 F3 0F 11 04 24 6A 00`; operates on whole
64-bit big-endian blocks, trailing `len % 8` bytes untouched. See `internal/paz/ice.go`.
BDO-LZ is a custom LZ variant — see `internal/paz/lz.go`.

Many `.dbss` tables are stored plaintext (no ICE, no LZ). If `compSize == origSize`,
the bytes are already readable.

---

## 2. .bss / .dbss tables

Structured data tables. Records are byte-packed C++ object dumps: fixed scalars,
fixed arrays, and length-prefixed strings/lists in declaration order. Production
decoders walk each record with `bss.Cursor` (see `internal/bss/cursor.go`). Offset
indexes are iterated with `bss.IndexedRecords` / `IndexedRecordsU16`
(`iter.Seq2[IndexedRecord, error]`).

- **PABR** (magic `PABR` = `0x52424150` at offset 0): the **string table is at the
  end**. The last 8 bytes are an `int64` pointer to it; at that pointer sits the string
  table (`[int32 count]` then per-string `[int32 len][bytes][sep]`). Records occupy
  `[8, stringTablePos)`. Open via `bss.OpenPABR`.
- **Non-PABR / offset-indexed** (e.g. `itemenchant.dbss`, `buff.dbss`): a sibling
  `*offset.dbss` supplies `[key, offset, size]` per row; strings are often inline
  (`i64` length + UTF-16/UTF-8 bytes). Variable or type-conditional layouts stay
  hand-written cursor walks inside those record boundaries.

Tables with **one uniform fixed record layout** can use `OpenPABR` + `RecordSize`
and a per-row cursor. Mixed layouts must not be forced into a flat field list.

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

Read positionally from @0 to @212 (`internal/tables/items.go`). Offsets are byte-exact
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
| 16 | byte×3 | extraSlots | front-packed occupied slots for multi-slot costumes; 46 = none |
| 19 | byte×43 | slot filler | always `0x2e` |
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
| 146 | u32 | unknown146 | 0 dominant; `0xffffffff` sentinel on some rows |
| 150 | u16 | unknown150 | 0 dominant; `0xffff` sentinel on some rows |
| 152 | u16 | unknown152 | 0 dominant; commonly 1000 when set |
| 154 | u32 | unknown154 | 0 dominant |
| 163 | byte | hideFromNote | `shownInNote = (== 0)` |
| 164 | bool | cash | pearl-shop item |
| 165 | byte | cronEnchant | |
| 168 | bool | dyeable | |
| 172 | byte | dyeParts | kept when 0 < v ≤ 30 |
| 196 | bool | personalTrade | |
| 197 | u16 | maxDurability | equipment only; large value = no-durability sentinel |
| 200 | byte | marketCategory id | → localization table 44 |
| 201 | byte | marketSubCategory id | |
| 202 | bool | nodeFreeTrade | |
| 204 | u32 | skillKey[0] | primary consumable skill → effect chain (§7) |
| 208 | u32 | skillKey[1] | additional skill; used by composite meals and a small number of other items |

Bytes not yet identified are captured as typed, deviation-only `ItemUnknowns`
(`unknown8`, `unknown85`, …). Constant runs are consumed and validated, so the
header ends exactly at @212 without relying on an anchor.

### `EItemType` and the tooltip label

The top-right item classification in the item tooltip comes from `itemType @4`, not
`EItemClassify @5`. The tooltip Lua reads `getItemType()` and selects these localized
GAME-sheet strings:

| Value | `EItemType` name | Tooltip label |
|---:|---|---|
| 0 | Normal | General |
| 1 | Equip | Equipment |
| 2 | Skill | Consumable |
| 3 | Tent | Holding Tool |
| 4 | Installation | Installable Object |
| 5 | Jewel | Socket Item |
| 6 | CannonBall | Cannonball |
| 7 | Mapae | License |
| 8 | Material | Crafting Material |
| 9 | Interaction | Enter Area |
| 10 | ContentsEvent | Special Items |
| 11–19 | ToVehicle / unidentified or reserved | General |

Trade goods add a contextual override: type 2 displays `Trade Item`; type 8 displays
`Crafting Material/Trade Item`; other types display `<label>/Trade Item`. The extractor
exports the numeric `ItemType` enum and its generated metadata supplies the ordinary
tooltip title. `ItemType.TooltipTitle(forTrade)` applies the trade override.

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

The first 18 bytes after `icEnd` are fixed. They are followed by three variable UTF-16
strings and then the market registration limit; reading the limit at a fixed icon
offset only works when all three strings are empty.

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | bool | marketable | may be listed on the Central Market |
| 1 | u8 | unknownIcon1 | |
| 2 | u8 | unknownIcon2 | defaults to 9 |
| 3 | u8 | unknownIcon3 | |
| 4 | byte×4 | reserved | zero |
| 8 | u8 | unknownIcon8 | cooking-product candidate |
| 9 | u8 | unknownIcon9 | |
| 10 | u8 | unknownIcon10 | food-tier candidate |
| 11 | byte×2 | reserved | zero |
| 13 | u8 | familyInventory | 2 permits Family Inventory; 0 does not |
| 14 | u8 | unknownIcon14 | |
| 15 | u8 | bindType | item binding behavior |
| 16 | u8 | unknownIcon16 | resource/source-class candidate |
| 17 | byte | reserved | zero |
| 18 | UTF-16×3 | client messages | three `[i64 chars][UTF-16LE]` strings |
| variable | i64 | marketRegisterLimit | follows the third string; positive values below 2³² are retained |

The limit is followed by an 84-byte fixed prefix:

| Relative @ from limit end | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | unknown | small enum |
| 1 | u32 | unknown | |
| 5 | u32 | unknown | |
| 9 | byte×3 | unknown enum array | front-packed |
| 12 | byte×43 | filler | always `0x77` |
| 55 | u8 | unknown | |
| 56 | u8 | unknown | flag-like |
| 57 | byte | reserved | zero |
| 58 | u8 | unknown | flag-like |
| 59 | u8 | unknown | small enum |
| 60 | u32×6 | unknown rates | commonly 1,000,000 |

The remaining type-dependent bytes are consumed and preserved as
`ItemUnknowns.UnknownPostIconTail` for Go consumers. They are excluded from JSON to
avoid duplicating hundreds or thousands of opaque bytes on every item.

- **Contribution-point cost** — search from `icEnd` for the 7-byte marker
  `13 06 00 00 00 00 13`; cost = `u32(marker + 20)`, kept when 1..1000 ("[CP]" rental
  gear and placeables).
- **Footer** — ends `[u32 self-id][u16 crystalGroup][u16 unknown]`; group `!= 0xFFFF`
  is the crystal transfusion group (name + max count from localization table 121).
  The final word currently uses 0, 1 and 256 and is not a constant marker.

---

## 4. `itemmaxlevel.dbss`

Via `itemmaxleveloffset.dbss` (key = item id). Each record is `[u32 id][u8 maxLevel]`.
The index has a zero size-column (fixed-stride records), so it's read directly.

---

## 5. `enchantstaticstatus.dbss` — enhancement curves

Per-(item-family, level) stat curve. Record key = `(enhanceLevel << 24) | baseId`, so
`baseId = key & 0xFFFF` and `level = key >> 24`. Key bits 16–23 are reserved and zero;
the current table uses levels 0–25. The record's first `u32` repeats `baseId`.

Each record is one front-to-back sequential field stream (no fixed offsets), read
largest-type-first — mostly `u32`, with a `u16` block at @53–60 (where a `u32` would
straddle two fields) and lone shift bytes at @24/@59. The meaningful fields:

| @ | Type | Field | Notes |
|---|---|---|---|
| 0 | u32 | baseId | == low 16 bits of the index key |
| 4 | u32×5 | unknown | typed values retained as `unknown4`…`unknown20` |
| 24 | u8 | unknown24 | flag-like |
| 25 | u32×4 | unknown | enhancement-process values retained as `unknown25`…`unknown37` |
| 41 | u32 | enhanceChance | value ÷ 1e6 = base success probability at this level (0 failstacks): 1.0 for +1–7, then falling. Also flags the scheme (below) |
| 45 | u32×2 | unknown | rate/process values |
| 53 | u16 | durability | base 100 → PRI 120 / DUO 140 / TRI 160 / TET 180 / PEN 200 |
| 55 | u16×2 | unknown | enhancement-process values |
| 59 | u8 | unknown59 | flag-like |
| 60 | u16 | unknown60 | enhancement-process value |
| 62 | f32 | maxHP | 0 unless the DSL carries `HP_UP(n)` |
| 66 | f32×25 | indexed species AP | populated slots are retained as `{index,value}` until the client enum is mapped |
| 166 | u8 | unknown166 | combat-stat lane flag |
| 167 | f32 tri-dice ×7 | AP / defense | slots: —, —, minAP, maxAP, displayAP, damageReduction, evasion |
| 251 | u32 | unknown251 | packed field before the strings |
| 255 | inline UTF-16 | sourceDescription | optional formatted Korean enhancement text, used by ship equipment |
| variable | inline UTF-16 | effect DSL | follows `sourceDescription`; length-prefixed (below) |

Each inline UTF-16 string is `[i64 code-unit count][UTF-16LE]`. The remaining header
scalars and display-tail rates are captured as typed, offset-named `EnchantUnknowns`.

**AP is three dice side by side** — melee, ranged, magic. A sword fills only melee, a
staff only magic; hybrids fill two equally — take the max across the three slots. The
display slot is the game's rounded `(min+max)/2`. `dp = evasion + damageReduction` (base
values).

**The display-stat tail** follows the DSL. It begins with 13 typed bytes
`[u8,u8,u32,u32,u8,u8,u8]`, including two commonly 1,000,000 and 700,000 rate fields.
Next is an accuracy block — 3× `[inline UTF-16 dice][f32 value]` — then a defense block
— 3× `[f32 evasion][f32 addedEvasion][f32 damageReduction][f32
addedDamageReduction]`. The three lanes are melee, ranged and magic. Accuracy and the
`+N` added-defense values appear only here; base evasion/DR duplicate the header.

The record footer is fully counted and bounded:

| Order | Type | Field | Notes |
|---:|---|---|---|
| 1 | i32×3 | sentinels | all three are `-1` |
| 2 | byte×65 | unknownTail12 | fixed structured block, preserved verbatim |
| 3 | u32 | enhancementAidCount | 0–3 in the current table |
| 4 | u32 × count | enhancementAids | valid item ids, including enhancement hammers and Crystals of Origin |
| 5 | byte×6 | unknownFooter | retained when nonzero |

**Effect DSL** — the second length-prefixed UTF-16 string after @251: a `;`-separated list of
`NAME(args)` formulas (item + set effects) — `HP_UP(110)`, `MON_DAM_REDUCE_ADD(10)`,
`NO_3_SET_EFFECT()`, `ALL_AP_INCRE()`. Parsing notes:

- Func names are usually SCREAMING_CASE but some are mixed-case (`Donkey_Harness_SET_EFFECT_1_2`).
- Args can be fractional (`ALCHEMY_REDUCE_TIME_DOWN(0.7)`) or roman numerals
  (`MERMAID_HOPE_ADD(IV)` = the tier).
- Argless funcs may be section markers, set markers, named capabilities, or display
  directives whose value comes from the containing row. Confirmed row-backed
  directives are exposed as `curveFields`:

| DSL function | EnchantLevel field(s) |
|---|---|
| `ALL_AP_INCRE`, `ALL_AP_INCRE_VALUE` | `ap` |
| `ALL_HIT_INCRE` | `accuracy` |
| `ALL_DP_INCRE` | `dp` |
| `ALL_EVA_INCRE` | `evasion`, `addedEvasion` |
| `ALL_DAM_REDUCE_INCRE` | `damageReduction`, `addedDamageReduction` |
| `HP_UP_16` | `maxHp` |
| `DUR_INCRE`, `MAX_INDURANCE_INCRE` | `durability` |

`NU_ALL_REG_ADD()` and `KU_ALL_REG_ADD()` are fixed client constants displayed as
All Resistance +10%. Both also represent the All Special Attack Extra Damage +10%
granted to Nouver and Kutum sub-weapons; neither co-occurs with the explicit
`ALL_SPECIAL_ATT_DAM_ADD` encoding. The additional output line uses the canonical
`ALL_SPECIAL_ATT_DAM_ADD(10)` function and retains the raw marker in `derivedFrom`,
allowing stat routers to use the ordinary special-attack mapping while preserving
whether it came from Kutum or Nouver. The raw marker argument list remains empty; the generic
`ALL_REG_ADD(n)` carries its value in the argument. Other argless functions remain
argless unless their value source is independently identified. The
`curveFields` interpretation applies only when the function is argless. A function
with an explicit argument retains that magnitude; `cronenchant.bss`, for example,
emits `ALL_EVA_INCRE(8)` as a normal +8 effect.

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

### `skillpiece.dbss` — item-set definitions

This table supplies the item-set headings and piece-count bonus text used by the UI.
It is indexed by `skillpieceoffset.dbss`; the index key is the record's `skillNo`.
There are no skipped or padding bytes in a valid record.

| Order | Type | Field | Notes |
|---|---|---|---|
| 1 | u32 | skillNo | matches the offset-index key |
| 2 | u32 | bonusCount | number of set-bonus tiers |
| 3 | u32 | firstPieces | piece count for the first tier |
| 4 | repeated | bonuses | first tier begins with `u16 apply`; later tiers begin with `u32 pieces, u16 apply` |
| 5 | u32 | footer | always zero |

Each bonus then carries three inline strings in order: `groupTitle`,
`descriptionTitle`, and `description`. Each string is `[i64 UTF-16 code-unit count]`
followed by that many UTF-16LE units. `pieces` is the equipped-piece threshold;
`apply` is the client's tier/application ordinal exposed by Lua as `getApply()`.

Localized versions are in `.loc` table **52**, with `id = skillNo`. The low 24 bits
of the field selector are `apply`; its high byte selects which string is requested:

| Field selector | String |
|---|---|
| `apply` | description |
| `0x01000000 \| apply` | description title / piece label |
| `0x02000000 \| apply` | group title |

Every non-empty inline source string has a corresponding table-52 localization. Empty
source tiers also have no localization entry and should remain empty.

The table defines bonuses, but does **not** contain a universal list of member item
ids. Some life-accessory families expose the relation in enhancement DSL functions:

| DSL function | skillNo | Family |
|---|---:|---|
| `ACCSET_1GRADE_LIFE_EXP_POINT_ADD` | 57991 | Loggia |
| `ACCSET_2GRADE_LIFE_EXP_POINT_ADD` | 57992 | Geranoa |
| `ACCSET_5GRADE_LIFE_EXP_POINT_ADD` | 57993 | Manos and Preonne combined |
| `ACCSET_6GRADE_LIFE_EXP_POINT_ADD` | 57551 | Preonne |

Several families mint a distinctive section-marker prefix in the item enhancement
DSL. These provide locale-independent membership even though the record contains no
numeric `skillNo` foreign key:

| DSL marker prefix | skillNo | Family |
|---|---:|---|
| `GBEAR_` | 47639 | Bear Necessities |
| `BLACKSTAR_` | 52494 | Blackstar armor |
| `ANCIENT_`, `EDANA_` | 57337 | Slumbering Origin and Edana defense gear |
| `SET_DECORATE_Training` | 57482 | Venia Riding attire |
| `DEBOREKA_` | 58080 | Deboreka accessories |
| `TUNGRAD_` | 58454 | Tungrad accessories |

Generic markers such as `NO_2_SET_EFFECT`, `NO_3_SET_EFFECT`, and `SET_EFFECT`
are reused by unrelated families. Neither `skillpiece` nor the item tables expose a
universal numeric relation for those definitions, so the marker alone cannot identify
their `skillNo`. Do not match localized description text: wording and stat names vary
by language. Families without a distinctive marker require another confirmed client
relation or a small explicit, auditable member list.

### `lightstoneset.bss` — artifact/lightstone combinations

This PABR table defines the named bonuses activated by combinations of three or four
lightstones in an artifact setup. The row key is also the id in localization table
**113**. The referenced skill supplies the actual effects through the normal
`skill.dbss` → `buff.dbss` chain.

Rows are packed consecutively without an item-count field:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | key | combination id; loc table 113 id |
| 4 | u32 | skillKey | low 16 bits identify the skill |
| 8 | u16 | reserved | zero |
| 10 | u32 ×3 | requiredItems | first three required item ids; duplicates are meaningful |
| 22 | u32 | fourthItem | present only for four-lightstone combinations |
| 22 or 26 | u32 | descriptionIndex | embedded Korean UTF-16 string-table index |

There is no explicit discriminator for the optional fourth item. Item ids and string
indexes occupy disjoint key spaces: the next `u32` is the fourth item when it is not a
valid string index. The corresponding `skilloffset.dbss` key is
`(uint16(skillKey) << 16) | 1`, where `1` is skill level one.

After all rows is a global item-equivalence map:

| Order | Type | Field | Notes |
|---|---|---|---|
| 1 | u32 | aliasCount | number of pairs |
| 2 | repeated u32 ×2 | item / countsAs | alternate item and the canonical requirement it satisfies |

The map is how amplified lightstones satisfy requirements written for their ordinary
counterparts. It also contains identity pairs. Some aliases can reference client
records omitted from a localized item table. The output therefore preserves numeric
`itemId`/`countsAsId` values for every pair and adds item URNs only when resolvable.

`enchantlightstone.bss` and `lightstoneenchantgroup.bss` are valid but empty PABR
tables in the current client; individual artifact and lightstone stats are carried by
their ordinary item/enhancement records.

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
| 7 | f32×7 | added stats | getter order: DD (AP), HIT (accuracy), DV (evasion), HDV (hidden evasion), PV (DR), HPV (hidden DR), MaxHP |
| 35 | u32 | added MaxMP | cumulative Max MP/WP/SP; small integer, not a float |

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
`enhancement.levels[].caphras`. Each step exposes the eight columns directly under
`stats` and also emits them as DSL effects in the same `{func, args}` shape as
enhancement effects. The two hidden columns use extension names
`HIDDEN_EVA_INCRE` and `HIDDEN_DAM_REDUCE_INCRE`. These DSL functions carry explicit
arguments and therefore do not use `curveFields`.

Why it's easy to miss: the totals are cumulative u32s spaced 39 bytes apart, the stat
ramps are floats, and the system is named "cron", not "caphras" — so find a system by
its internal name (via the UI lua getters) and scan value sequences at a candidate
record's stride, not contiguously.

---

## 7. Consumable effect chain (item→skill→buff)

Food and elixirs use `ItemTypeSkill` (`itemType = 2`): using one casts a skill that applies buffs.
The data is a three-table chain (`internal/tables/buffs.go`). A consumable's skill keys
(`u32[2] @204/@208` in the item row) indexes `skilloffset.dbss` → records in `skill.dbss`, which
carries the cooldown (`u32 @95`, ms; kept when >0, ≤1e8, %1000==0) and a `u16` buff-index
list from `@99` (read until a 0, or an index absent from `buff.dbss`). Effects from both
skill slots are combined in slot order. Each index →
a `buff.dbss` record. A skill therefore applies several buff records; a module does not
contain or grant skills. Each buff has one `ModuleType` selecting how its arguments are
interpreted.

### `buff.dbss` schema

`buffoffset.dbss` is a PABR index with 10-byte rows `[u16 key, u32 offset, u32 size]`.
Its 44,295 entries tile `buff.dbss` exactly after the four-byte row count, and every
index key equals the `Index` at the start of its record. The index supplies the record
boundaries; fields are then consumed sequentially because inline strings make the
records variable-length:

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
| i32 | DurationMs | milliseconds; negative values are preserved by the decoder |
| [25 bytes] | UnknownDuration | application/timing fields, unmapped |
| Text | ApplyToGroup | |
| UtfText | Icon | UTF-8 path |
| u8 | UnknownIconByte | unmapped |
| i32 | UnknownIconValue | unmapped |
| Text | Desc | |
| [24 bytes] | UnknownTail0To23 | unmapped trailing configuration |
| u8 | StackingCategory | broad buff family/control category |
| u8 | UnknownTail25 | normally 1 for categorized effects |
| u8 | UnknownTail26 | unmapped |

`buffsimply.bss` is a 30-byte-per-row PABR projection over the same 44,295 buff keys.
It includes the icon string-table reference but not the complete effect arguments, so
the full `buff.dbss` record remains the source for effect decoding.

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
| 63 | (amount) | immediate Worker Stamina recovery |
| 29 | (amount×10000) | Weight Limit (LT) |
| 9/10/11/30/50/57 | (amount%) | Move/Attack/Cast Speed, Crit Rate, Mount EXP, Drop Rate |
| 25 | (amount%, kind, lifeSkill) | kind 0 Combat / 1 Skill / 2 life-skill EXP |
| 39/40/41/43 | (target, amount) | target 0 melee / 1 ranged / 2 magic / 3 all — AP/Acc/Eva/DR |
| 46 | (species, amount) | extra AP vs Humans/Demihumans/Beasts/… |
| 49/105 | (kind, amount%) | CC resistance / ignore-resistance |
| 67 | (kind, ranks) | potential slots |
| 93 | (kind, amount%) | special-attack extra damage |
| 80 / 149 | (lifeSkill, amount) / (lifeSkill, _, amount) | life-skill EXP / mastery |
| 79 | (amount) | immediate Energy recovery |
| 89 | (fitness, amount, rule) | immediate Breath / Strength / Health EXP |
| 90 | (amount%) | Death Penalty Resistance |
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

Module 58 is the composite/preset record used by draughts, perfumes and Cron meals. Its
description is the multi-line headline shown by the client, while sibling buff rows
carry the individually structured effects. Item rows contain two fixed skill keys;
composite meals distribute their component buffs across both skill records. Some
components also have public skills keyed `(buff.Index << 16) | 1`, but those are not the
item's link. Combining both item skill slots recovers the complete Cron-meal effects
without parsing localized descriptions.

Module 184 is the meal's Satiated marker. Module 38 appears on internal/proxy consumable
records and its argument is not an item foreign key. Neither is emitted as a stat
modifier.

### Buff replacement and consumable families

`Group` and `StackingCategory` describe different scopes. `Group` is the narrow
replacement key for variants of one effect: food buffs of different grades and
durations that grant Max HP all use group 5616, for example. Consumers should keep at
most the active member of a nonzero group rather than summing two members of that same
group. Across linked consumables, all 130 shared groups whose modules resolve contain
one canonical stat type; none mix unrelated stats (four additional groups contain only
unresolved marker modules).

`StackingCategory` is a broader family/control byte shared by every component of a
consumable. Its confirmed values are:

| value | family/control | evidence |
|---:|---|---|
| 1 | food | ordinary meals and all their component buffs |
| 2 | elixir/draught | ordinary elixirs and every draught component |
| 6 | perfume | every perfume component |
| 10 | Cron/special-meal secondary effect | secondary meal skill and its extra components |
| 21 | whale-tendon elixir | Whale Tendon, Tough Whale Tendon and Sturdy Whale Tendon effects |
| 26 | draught reset control | unique module-58 `영약 추가 효과 초기화` record |

Draught items carry a second fixed skill applying the category-26 control buff. That
control clears category 2, explaining why the last draught replaces other draught and
ordinary elixir effects while category-6 perfumes and category-21 whale-tendon elixirs
remain. `Effects.buffCategories` contains every nonzero category in the complete item
buff chain, including non-stat control records; a draught therefore carries `[2,26]`,
a perfume `[6]`, and a Cron meal `[1,10]`. `Effects.clearsBuffCategories` contains the
families removed on use, so it contains `[2]` for a draught. Each emitted stat also
retains `buffGroup` and `buffCategory`, allowing consumers to apply both the narrow
replacement rule and broad reset rule without parsing tooltip text. `ConditionType`
is unrelated to this mechanism; it selects triggers such as on-hit or
on-critical-hit recovery.

Fallbacks: a module not in the table falls back to the localization-table-5 English
name parsed into `{stat, op, value, unit}`; a buff with neither is kept as a hidden
Korean-named effect when it can be parsed. Text parsing alone under-counts consumables
because many component buffs are Korean-only, so the binary module decode is primary.
Resolved effects also carry a canonical `statId` where the shared stat model has an
equivalent accumulator key; `buffModule` remains available for traceability. Each
emitted stat preserves its own duration and whether it is immediate;
the item-level duration is only the longest timed component. `BuffType` is not a usable
shown/hidden flag (it is `1` for nearly everything).

### Player skill groups and passive stats

Player abilities are split across a rank-chain table, per-class UI grids and the
ordinary skill→buff effect chain. This is enough to associate a passive with a class,
select its learned rank and apply its canonical stat modifiers without parsing tooltip
text. A selected rank replaces the earlier rank; the rank effects are not cumulative.

`skillgroup.bss` is a non-PABR rank map:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | groupCount | |
| row +0 | u16 | groupNo | joins UI skill-grid cells |
| row +2 | u32 | rankCount | includes the unlearned entry |
| row +6 | u32 ×rankCount | skillKeys | first key is 0; remaining keys pack `skillNo << 16 \| skillLevel` |

`ui_skillgroup_combat.bss` and `ui_skillgroup_awakening.bss` are PABR tables with one
variable row per class. Their grid cells tile the record stream exactly:

| @ | Type | Field | Notes |
|---:|---|---|---|
| row +0 | u8 | classType | playable-class enum |
| row +1 | u32 | width | grid columns |
| row +5 | u32 | height | grid rows |
| cell +0 | u32 | typeCount | number of drawing/cell types |
| cell +4 | u8 ×typeCount | types | type 2 means the cell owns a skill group |
| — | u16 | groupNo | low half of the packed cell value |
| — | u8 | unknown | middle byte of the packed cell value |
| — | u8 | subGroup | high byte of the packed cell value |

The grids are row-major and preserve blank/line cells as well as skill cells, so they
can reproduce the class skill-tree layout. A subgroup directory follows all class
grids; its string keys are readable, but its internal entries remain preserved as an
unknown footer.

`skilltype.dbss` supplies each rank's identity. The paired offset index points past a
repeated four-byte row key; the record itself begins:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | skillKey | matches the offset-index key |
| 4 | inline UTF-16 | sourceName | Korean skill/rank name |
| — | inline UTF-16 | sourceGroupName | Korean family name; may be empty |
| — | u32 | kind | 0 unknown/internal, 1 active, 2 passive |
| — | variable | action configuration | animation, icon, presentation and combat behavior; not decoded here |

Localization table 10 maps `skillNo` to the displayed name and description. Passive
rank effects use the same `skill.dbss` → `buff.dbss` chain described above. For example,
the client data resolves Sword Training XX to All AP +1 and All Evasion +2, Precise
Martial Arts to All AP +5, and Infinite Mastery VI to Max HP +200 and All Accuracy +18.

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
| 10 | skill names and descriptions, keyed by skill number |
| 12 | territories — field 0 = nation, description = territory name |
| 17 | topography — place / region names |
| 18 | quests — name, description, giver, objective |
| 29 | worldmap node names |
| 34 | knowledge card name / description / acquisition |
| 37 | compiled UI string sheets (`GAME`, `RESOURCE`, `ACTIONCHART`, etc.) |
| 44 | central-market categories (see below) |
| 115/116/117 | Monster Zone Info sub-category / zone / tag names |
| 121 | crystal transfusion group — id = group, key1 = max count, text = name |
| 123 | workshop / house names (by `eHouseIconType`) |

### Crystal transfusion rules

An item's footer carries its crystal group number. Localization table 121 supplies the
display name and limit, while `jewelgroupstaticstatus.bss` independently defines the
same limit and is used as the structural source of truth:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | sourceNameIndex | index into the table's Korean UTF-16 string table |
| 4 | u16 | groupNo | joins the item footer and loc table 121 |
| 6 | u16 | maxCount | 1000 represents no practical limit |

`jewelspecialslotsgroupstaticstatus.bss` restricts newer preset slots to particular
crystal groups. Its variable rows are `[u8 specialSlot][u32 count][count × u16
groupNo]`. Confirmed special-slot values are 14 costume armor, 17 necklace, 18/19
rings, 20/21 earrings and 22 belt. Costume armor accepts Ancient Spirit group 101;
the six accessory slots accept Dawn group 103. The extracted `crystal_rules.json`
keeps the numeric relations so consumers do not need localized-name matching.

Internal table text (Name fields in `.dbss`) is **Korean** even on the EU client; the
display text is resolved through `.loc` by id. Searching the binaries for English finds
nothing — search Korean (UTF-16).

### `stringtable.bss` — symbolic UI keys

Lua calls such as `PAGetString(Defines.StringSheet_GAME, "LUA_...")` use symbolic
keys, while localization table 37 stores numeric ids. `gamecommondata/binary/stringtable.bss`
bridges the two: each symbolic key is paired with the numeric hash used as the
localization id and with its Korean source text.

The file is PABR with eight variable records, 48,673 symbolic entries and a shared
UTF-16 string table. Each record is one string sheet:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | sheetHash | Hash of the sheet name |
| 4 | u32 | sheetNameStrIdx | String-table index, e.g. `GAME` or `RESOURCE` |
| 8 | u32 | entryCount | Number of entries in this sheet |
| 12 + n×16 | u32 | localizationId | Primary id in localization table 37 |
| 16 + n×16 | u32 | symbolicKeyStrIdx | String-table index of the `LUA_...`/`PANEL_...` key |
| 20 + n×16 | u32 | sourceTextStrIdx | String-table index of the Korean source text |
| 24 + n×16 | u32 | reserved | Zero |

The sheet normally selects the secondary field in localization table 37:

| Sheet | Field |
|---|---:|
| `CUTSCENE` | 0 |
| `GAME` | 1 |
| `RESOURCE` | 2 |
| `ACTIONCHART` | 3 |
| `TOOL` | 4 |
| `WEB` | 5 |
| `SymbolNo` | 6 |
| `IMAGESLIDE` | 7 |

Resolve an entry by `(localizationId, sheet field)` in localization table 37. If that
base field is absent, try `field | 0x10000`; the packed alternate is required by a
small number of `GAME` and `RESOURCE` entries. If neither exists, retain the Korean
source text from `stringtable.bss`. The optional `lua-strings` command performs this
join and writes `lua_strings_<lang>.json`; the normal build does not run it.

### Playable character classes

Playable classes use two different identifiers. `CharacterKey` identifies the player
prototype in `characterstatic.dbss` and localization table 6. `ClassType` is the
gameplay enum returned by `getClassType()`; it is also the bit position in an item's
class-restriction mask. They are not interchangeable: Ranger is CharacterKey 2 but
ClassType 4.

`pcgrowthsimply.bss` is the direct active-class list. It is a PABR table with 47
fixed 10-byte rows and a UTF-16 string table. The stored archive entry has an extra
ICE layer.

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | classType | Gameplay enum and item-mask bit |
| 1 | u32 | sourceNameStringIndex | Korean class name in the table string pool |
| 5 | u8 | playable | 1 for the 31 playable classes; 0 for reserved/test slots |
| 6 | u32 | unknown6 | Zero in every row; retained if a future row uses it |

`pcgrowth.dbss` contains the full ClassType → CharacterKey relation plus class
selection data. Its companion `pcgrowthoffset.dbss` begins with a `u32` row count,
followed by 9-byte index rows:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | classType | Record key |
| 1 | u32 | oneBasedOffset | Subtract 1 before slicing `pcgrowth.dbss` |
| 5 | u32 | sizeMinusOne | Add 1 for the record byte length |

The confirmed prefix of each variable `pcgrowth.dbss` record is:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | classType | Matches the offset-index key |
| 1 | u8 | classTypeCopy | Exact duplicate of `classType` |
| 2 | u16 | characterKey | Joins `characterstatic.dbss` and localization table 6 |
| 4 | u16 | unknown4 | Small class-selection value |
| 6 | u16 | unknown6 | Small class-selection value |
| 8 | u16 | unknown8 | Small class-selection value |
| 10 | u32 | unknown10 | Packed/high-valued class-selection value |
| 14 | u8 | unknown14 | Small class-selection value |
| 15 | u32 | starterWeaponCount | Count for the following item keys |
| 19 | u32 × starterWeaponCount | starterWeaponItems | Low-tier weapon item ids for the class |
| 19 + n×4 | 94 bytes | unknownConfiguration0 | Invariant opaque prefix, retained verbatim |
| 113 + n×4 | u8 | unknownConfiguration94 | Values 0–3 across the 47 class slots |
| 114 + n×4 | 4 bytes | unknownConfiguration95 | Invariant opaque suffix, retained verbatim |
| 118 + n×4 | inline UTF-16 | sourceName | `[i64 characterCount][UTF-16LE]` |
| variable | inline UTF-16 | sourceDescription | Korean class-selection description |
| variable | inline UTF-16 | selectionMovie | Class-selection `.webm` asset path |

The tail after `selectionMovie` is fully tiled. It starts with the gender flag and
four potion/food consume animations, each encoded as `[i64 byteCount][UTF-8]`:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | gender | 0 = male, 1 = female |
| 1 | inline UTF-8 ×4 | consumeAnimations | Level 1 through 4 consume action names |

After those variable strings is a common 71-byte presentation block. Offsets in this
table are relative to the start of that block:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | f32 ×6 | unknownPresentation0 | Class-specific values; retained as floats |
| 24 | u8 | unknownPresentation24 | Small enum/flag |
| 25 | u32 ×3 | previewWeaponItems | Main, sub and awakening weapon item ids used by class presentation |
| 37 | u16 | unknownPresentation37 | Usually 1 |
| 39 | f32 ×7 | unknownPresentation39 | Class-specific presentation values |
| 67 | u32 | unknownPresentation67 | Usually zero |
| 71 | u32 ×4 | unknownPresentationExtra | Present only for Shai: `[31, 4, 31, 0]` |

Two weapon-asset lists follow. Each list is
`[u32 count][count × {u8 slot, inline UTF-8 path}]`. Current playable rows contain
three assets per list; slots 1, 2 and 3 correspond to the main, sub and awakening
weapon models. The two lists are currently identical, but both are retained because
their separate purpose is not identified. All 47 indexed records are consumed to
their exact byte boundaries; 31 are playable and 16 are reserved or test slots.

`playercharacterstatic.bss` is a PABR membership list:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u16 | characterKey | Joins `characterstatic.dbss` and localization table 6 |

The table contains 96 player-like prototypes, including live classes, reserved/test
characters, mercenaries and alternate modes. It is therefore not an active-class list
by itself. In each matching `characterstatic.dbss` record, the direct relation is:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| record end - 23 | u32 | classType | CharacterKey → ClassType; verified for every active class |

`classskilllist.bss` provides the active playable ClassTypes. It is a variable-record
PABR table whose 31 rows tile the record area exactly:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | classType | Active playable class enum value |
| 1 | u64 | skillCount | Number of following skill keys |
| 9 | u16 × skillCount | skillKeys | Small curated class skill list, not the complete skill tree |

Filtering `pcgrowth.dbss` through the `pcgrowthsimply.bss` playable flag produces the
class identity map without treating reserved ClassType slots as playable:

| ClassType | CharacterKey | Display name |
|---:|---:|---|
| 0 | 1 | Warrior |
| 1 | 6 | Hashashin |
| 2 | 7 | Sage |
| 3 | 8 | Wukong |
| 4 | 2 | Ranger |
| 5 | 9 | Guardian |
| 6 | 10 | Scholar |
| 7 | 11 | Drakania |
| 8 | 3 | Sorceress |
| 9 | 12 | Nova |
| 10 | 13 | Corsair |
| 11 | 14 | Lahn |
| 12 | 4 | Berserker |
| 15 | 17 | Maegu |
| 16 | 5 | Tamer |
| 17 | 18 | Shai |
| 19 | 20 | Striker |
| 20 | 21 | Musa |
| 21 | 22 | Maehwa |
| 23 | 24 | Mystic |
| 24 | 25 | Valkyrie |
| 25 | 26 | Kunoichi |
| 26 | 27 | Ninja |
| 27 | 28 | Dark Knight |
| 28 | 29 | Wizard |
| 29 | 30 | Archer |
| 30 | 31 | Woosa |
| 31 | 32 | Witch |
| 32 | 33 | Seraph |
| 33 | 34 | Dosa |
| 34 | 35 | Deadeye |

The exact class-selection text and presentation data live in
`luacscript/x64/include/global_newclass_data.luac`. Its class-indexed tables include
main/sub/awakening weapon labels, combat-resource labels, awakening and succession
descriptions, combat style, weapon categories, class icons, class-selection stat
textures and content-group gates. Symbolic `PAGetString` keys in that Lua resolve
through `stringtable.bss` and localization table 37. This is the source of UI spelling
such as `Sorceress`; localization table 6 calls the underlying prototype `Sorcerer`.

### Character fitness progression

`fitnesslevel.dbss` stores the family-wide Breath, Strength and Health curves. It
begins with `u32 kindCount = 3`; each kind is `[u32 levelCount][level records]`.
There are 51 levels, numbered 0 through 50, for each kind.

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | kind | 0 = Breath, 1 = Strength, 2 = Health |
| 1 | u32 | level | Fitness level |
| 5 | u32 | requiredExperience | Experience required for this level |
| 9 | f32 | unknown9 | Zero in every row; emitted if a future record uses it |
| 13 | f32 | maxStamina | Breath stamina bonus |
| 17 | f32 | maxWeight | Internal weight units; divide by 10,000 for LT |
| 21 | f32 | maxHP | Health HP bonus |
| 25 | f32 | maxMP | Health MP/WP/SP bonus |

`fitnessleveloffset.dbss` contains three concatenated offset indexes, one per kind.
Each index is `[u32 count][count × {u32 level, u32 offset, u32 size}]`; every record
is 29 bytes.

### Character level rules

`experience.bss` stores generic rules indexed by ClassType and character level. Its
PABR record area contains 47 counted class groups, one for every stored class slot 0
through 46. Each group is `[u32 levelCount = 131][131 x 228-byte records]` and covers
levels 0 through 130.

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | classType | Matches the containing class-group index |
| 4 | u32 | level | Character level, 0 through 130 |
| 8 | 20 bytes | reserved | Zero |
| 28 | 200 bytes | characterLevelStat | Shared character-stat structure described below |

The 200-byte stat structure is also the complete body of each row in
`hardcorecharacterstatstaticstatus.bss`. After its `PABR` marker it has 47 fixed
201-byte rows, each keyed by a leading u8 ClassType; all 47 bodies match the level-60
`experience.bss` stat structure exactly.
Offsets below are relative to the stat structure, so add 28 for their record offsets:

| Stat @ | Record @ | Type | Field | Notes |
|---:|---:|---|---|---|
| 0 | 28 | u32 | reserved | Zero |
| 4 | 32 | f32 | unknownStat4 | 1 in every row |
| 8 | 36 | u32 | unknownStat8 | 1 in every row |
| 12 | 40 | 20 bytes | reserved | Zero |
| 32 | 60 | f32 | unknownStat32 | 1 in every row |
| 36 | 64 | 24 bytes | reserved | Zero |
| 60 | 88 | u32 | unknownStat60 | 0 at level 0, otherwise 5 |
| 64 | 92 | u32 | unknownStat64 | 0 at level 0, otherwise 5 |
| 68 | 96 | u32 | unknownStat68 | 0 at level 0, otherwise 5 |
| 72 | 100 | 8 bytes | reserved | Zero |
| 80 | 108 | f32 | unknownStat80 | 1 in every row |
| 84 | 112 | 16 bytes | reserved | Zero |
| 100 | 128 | u32 | unknownStat100 | 0 at level 0, otherwise 5 |
| 104 | 132 | u32 | unknownStat104 | 0 at level 0, otherwise 5 |
| 108 | 136 | u32 | unknownStat108 | 0 at level 0, otherwise 5 |
| 112 | 140 | f32 | unknownStat112 | 1 in every row |
| 116 | 144 | f32 | apBonus | 0 below level 60, then 1 |
| 120 | 148 | f32 | dpBonus | 0 below level 56, then 1 |
| 124 | 152 | 76 bytes | reserved | Zero |

The AP and DP fields are the one-time level milestones shown by the client: level 56
grants 1 DP and level 60 grants 1 AP. Corresponding records for different classes are
byte-identical apart from `classType`, so the table does not provide class-specific HP,
resource or weight curves. The two repeated raw-value triads at stat offsets +60 and
+100 change from 0 at level 0 to 5 from level 1 onward. Their placement and the
client's `lvdd`/`lvpv` terminology make attack/defence growth plausible, but that
interpretation is not yet strong enough to expose as a named gameplay stat.

The class groups end exactly 200 zero bytes before the PABR string table.

### Adventure journals and family stats

`journalquest.dbss` describes the adventure bookshelf: journal groups, books and the
ordered quest ids used as pages. Its first `u32` is the journal-group count. Each
group begins with a `u32 bookCount`, followed by its variable-length book records.
`journalquestoffset.dbss` supplies the grouping and exact record boundaries:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | groupCount | Number of following journal groups |
| 4 | repeated | groups | `{u32 groupKey, u32 bookCount, bookCount × BookIndex}` |

| BookIndex @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | bookKey | Book number within the group |
| 4 | u32 | offset | Absolute offset in `journalquest.dbss` |
| 8 | u32 | size | Exact record length |

Book-index order is UI order and need not be physical file order. The indexed records,
plus one four-byte book count per group, tile the data file exactly.

| Book record field | Type | Notes |
|---|---|---|
| journalKey | u32 | Matches the containing group |
| bookKey | u32 | Matches the offset-index key |
| unknown8 | u8 | Boolean; set on several narrative journals |
| sourceJournalName | inline UTF-16 | Korean source title |
| sourceJournalDescription | inline UTF-16 | Korean source description |
| sourceBookName | inline UTF-16 | Korean volume title |
| sourceRequirement | inline UTF-16 | Korean unlock requirement |
| icon | inline UTF-8 | Bookshelf icon asset |
| texture | inline UTF-8 | Book texture asset |
| pageCount | u32 | Number of following packed quest ids |
| pageQuests | u32 × pageCount | `(questIndex << 16) | questGroup` |
| reservedEnd | u32 | Zero |

Localization table 63 is keyed by `journalKey`; the low 24 bits of its packed field
select `bookKey`, while the high byte selects the presentation field:

| Field plane | Localized value |
|---:|---|
| 0 | Parent journal name |
| 1 | Parent journal description |
| 2 | Book unlock requirement |
| 3 | Book name |

The output uses these localized values for the selected client language and falls
back to the embedded Korean fields when a translation is absent.

The packed ids join localization table 18 for each page's translated quest name,
description, giver and objective. They also join the corresponding variable record in
`quest.dbss`. Journal-page quest records have a stable 128-byte prefix: the packed id
at +0, twelve zero reserved bytes at +8, and a quest-kind word at +20 whose low byte is
7. The remaining +24..+127 prefix is retained as unknown data. Every referenced page
has exactly one record matching that structure.

`allquestlist.bss` is a PABR array of packed quest ids in the same physical order as
the variable records in `quest.dbss`. The condition sub-record near the end of each
ordinary quest has the following shape. Its self id and the next ordered quest id
provide structural record boundaries; condition text is executable client DSL rather
than localized prose.

| Condition tail field | Type | Notes |
|---|---|---|
| questId | u32 | `(questIndex << 16) \| questGroup`; repeats the owning record id |
| unknownHeader | 25 bytes | Fixed condition-tail configuration |
| acceptCondition | inline UTF-16 | Prerequisites such as `ClearQuest(...)`, level, class or item checks |
| completeCondition | inline UTF-16 | Completion expression such as `meet(...)` |
| unknownObjectivePrefix | variable bytes | At least 24 bytes; may contain a short counted section |
| sourceObjective | inline UTF-16 | Korean presentation text for the objective |
| unknownEnd | u32 | Final scalar; also marks the exact end of the record |

Condition expressions use semicolon-separated calls, comparisons, `!` negation and
markers such as `<or>`. Localization table 18 supplies the translated objective text;
it does not replace the condition DSL. The final entry in the current quest list is
an incomplete placeholder and has no condition tail.

Quest records contain five fixed-stride base-reward unions. Their family-stat
sub-block begins at `128 + rewardSlot × 178`, for reward slots 0 through 4. Journal
pages normally use slot 0; ordinary quests with permanent Family rewards may use a
later slot. The sub-block is byte-packed; in particular, `inventory` is one byte, so
the later fields are not four-byte aligned:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 128 | u32 | familyStatType | Selects the populated value field; 16 means no family-stat reward |
| 132 | f32 | offence | All AP |
| 136 | f32 | defence | All DP |
| 140 | f32 | hp | Max HP |
| 144 | f32 | mp | Max MP |
| 148 | i32 | stamina | Max Stamina |
| 152 | i32 | weight | Divide by 10,000 for LT |
| 156 | u8 | inventory | Inventory slots |
| 157 | f32 | accuracy | Accuracy |
| 161 | f32 | evasion | Evasion |
| 165 | i32 | enhancementChance | Enhancement Chance |
| 169 | i32 | valksLimit | Additional Enhancement Chance Limit |
| 173 | i32 | stackLimit | Enhancement Chance Stack Limit |

The type values are 0 through 11 in the same order as the value fields above. A real
reward populates only its selected field. Type 16 is the empty/default union. The same
structural quest subtype covers both bookshelf pages and the separate quests that
grant permanent Family stats. Reading all five slots therefore captures the complete
quest-backed source list as well as the bookshelf hierarchy; summing it produces the
client's permanent Family-stat totals.

### Life-skill types and progression

The life-skill wire value is the client's `CppEnums.LifeExperienceType`. The client's
raw enum names survive in `global_define_cpp_enum.lua` (the "Client raw name" column
below); localized display names come through the Lua string table. The
`LifeSkillType` enum normalizes its `nativeName` to the app's canonical skill key
(lowercase public name, e.g. `processing` not `manufacture`) so it lines up 1:1 with
the gear-builder's config keys and the per-skill mastery StatIds — each playable skill
carries a typed `masteryStat` (`gatheringMastery`, …). `Type_Count = 15` is a sentinel,
not a sixteenth skill. Some effect modules reuse that sentinel value to mean “all life
skills.”

| Wire | Public name | Client raw name | nativeName / masteryStat | Notes |
|---:|---|---|---|---|
| 0 | Gathering | `gather` | `gathering` / gatheringMastery | |
| 1 | Fishing | `fishing` | `fishing` / fishingMastery | |
| 2 | Hunting | `hunting` | `hunting` / huntingMastery | |
| 3 | Cooking | `cooking` | `cooking` / cookingMastery | |
| 4 | Alchemy | `alchemy` | `alchemy` / alchemyMastery | |
| 5 | Processing | `manufacture` | `processing` / processingMastery | |
| 6 | Training | `training` | `training` / trainingMastery | |
| 7 | Trading | `trade` | `trading` / tradingMastery | |
| 8 | Farming | `growth` | `farming` / farmingMastery | |
| 9 | Sailing | `sail` | `sailing` / sailingMastery | |
| 10 | Quest | `temp1` | `quest` | The current English UI labels this slot Quest |
| 11 | Bartering | `barter` | `bartering` | No gear mastery |
| 12 | Reserved | `temp2` | `temp2` | Current UI label: Temp |
| 13 | Reserved | `temp3` | `temp3` | Current UI label: Temp |
| 14 | Reserved | `temp4` | `temp4` | Current UI label: Temp |
| 15 | Count | `Type_Count` | — | Sentinel; also used as the all-life-skills selector |

The visible grade and grade-level are derived from the raw level. These boundaries are
defined directly by `PaGlobalFunc_Util_CraftLevelReplace` in the client Lua:

| Grade wire | Name | Raw levels | Displayed levels |
|---:|---|---:|---:|
| 0 | Beginner | 1–10 | 1–10 |
| 1 | Apprentice | 11–20 | 1–10 |
| 2 | Skilled | 21–30 | 1–10 |
| 3 | Professional | 31–40 | 1–10 |
| 4 | Artisan | 41–50 | 1–10 |
| 5 | Master | 51–80 | 1–30 |
| 6 | Guru | 81–180 | 1–100 |

#### `lifeexp.dbss`

This table is self-describing and contains the required-experience curve for every
`LifeExperienceType` slot. It tiles exactly as follows:

```text
[u32 typeCount = 15]
15 × {
    [u32 levelCount = 181]
    181 × LifeExperienceRow
}
```

| Row @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | lifeSkillType | Matches the containing group, 0–14 |
| 1 | u32 | level | Raw level, 0–180 |
| 5 | u64 | requiredExperience | Experience requirement for this row |

The curves are not all interchangeable: Fishing, Trade and Bartering differ from the
common curve. `lifeexpmaxlevel.bss` is a parallel flat array of 15 `u32` maximum
levels; every current entry is 180. The extractor writes these tables as
`life_skill_progression.json` and exposes `LifeSkillType`, `LifeSkillGrade` and
`LifeSkillLevel` publicly.

#### Life-skill mastery and level-effect tables

These client tables drive the values displayed by the Life Skill tab. Percentage-like
fields are stored as integer rates where `1,000,000 = 100%`.

| File | Shape | Confirmed purpose |
|---|---|---|
| `commonlifestatdata.bss` | extra-ICE PABR; 180 consecutive `f32` values | Base mastery contributed by raw life levels 1–180 |
| `collectingstatdata.bss` | 7 groups × `[u32 61][61 × 36-byte row]`, then 44 zero bytes | Gathering mastery for water, lumbering, fluid collection, hoe gathering, butchering, tanning and mining |
| `fishingstatdata.bss` | PABR; 61 × 8-byte row | Prize Catch rate by Fishing mastery |
| `huntingstatdata.bss` | PABR; 61 × 36-byte row | Hunting resource-quantity rate plus seven unused/reserved columns |
| `cookingstatdata.bss` | PABR; 61 × 24-byte row | Mass cooking, max products, higher-grade products and Imperial Delivery bonus |
| `alchemystatdata.bss` | PABR; 61 × 40-byte row | Max products, extra-result tiers and Imperial Delivery bonus |
| `manufacturingstat.bss` | 6 identical groups × `[u32 76][76 × 16-byte row]` | Processing proc rate and mass-process batch size |
| `trainingstatdata.bss` | PABR; 61 × 16-byte row | Horse capture, mount EXP and higher-tier breeding rates |
| `sailstatdata.bss` | PABR; 61 × 44-byte row | Ship acceleration, speed, turn and brake bonuses, plus preserved step/configuration fields |
| `barterlifelevelinfo.bss` | PABR; 180 × 8-byte row | Parley-cost reduction by raw Bartering level |

All mastery rows begin with an `f32 mastery` threshold. The three Gathering resource
tiers each occupy `{u32 dropRate, u32 quantityRate}` after that threshold; the final
two `u32` fields remain unidentified. Processing rows are
`{f32 mastery, u32 procRate, u32 batchSize, u32 reservedZero}`. The six Processing
groups correspond to Shaking, Grinding, Chopping, Drying, Filtering and Heating and
are byte-identical in the current client.

`lifeactionexperience.dbss` and `actionexp.dbss` are additional offset-indexed action
award tables, not the level requirement curve. Their record payloads do not follow the
ordinary decoded DBSS/PABR forms yet, so their per-action meanings remain open.

### Central-market categories — localization table 44

Keyed by the main-category id (item `@200`). Within each entry: `key1 == 0` is the main
name ("Consumables"); `key1` in `1..0xFFFF` are the sub-category names (matching item
`@201`); `key1 ≥ 0x10000` are per-category enhancement-level display labels (skipped).

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

Every map region (1,572): key, names, territory membership, world positions, flags and
warehouse groups. The file is PABR with a UTF-16 string table. Each variable record is
a 210-byte head, two counted lists and a 171-byte tail. Record size is
`389 + 2×warehouseGroupCount + 12×extraPositionCount`; the odd fixed size means fields
do not remain naturally aligned between records.

The 210-byte head is:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u16 | regionKey | Joins localization table 17, `regionclientdata` and `region_info.xml` |
| 2 | u8 ×3 | mapColor | RGB world-map color |
| 5 | u8 | reserved | Zero |
| 6 | u8 | regionType | Region category; 1 is a town/city and 2 is a field region |
| 7 | u8 | villageSiegeDay | `CppEnums.VillageSiegeType`: Sunday=0 through Saturday=6; 7 means none |
| 8 | u8 ×3 | reserved | Zero |
| 11 | u8 | unknown11 | Small enum; observed values 0, 1, 3 and 4 |
| 12 | bool | unknown12 | Unidentified flag |
| 13 | bool | unknown13 | Unidentified flag |
| 14 | bool | ocean | Open-ocean region |
| 15 | bool | desert | Desert region |
| 16 | bool | prison | Prison region |
| 17 | bool | sea | Sea region |
| 18 | bool ×9 | unknown18..26 | Unidentified flags, retained individually |
| 27 | bool | locator | Region participates in the client locator |
| 28 | bool | unknown28 | Unidentified flag |
| 29 | u16 | unknown29 | Unidentified value |
| 31 | bool | unknown31 | Unidentified flag |
| 32 | u32 | unknown32 | Observed as 19950 in all rows; not a record anchor |
| 36 | u8 | reserved | Zero |
| 37 | bool | unknown37 | Unidentified flag |
| 38 | u32 | villainRespawnWaypointKey | Outlaw/death fallback world-node key |
| 42 | f32 ×3 | villainRespawnPosition | Position paired with the fallback node |
| 54 | bool ×5 | unknown54..58 | Unidentified flags, retained individually |
| 59 | u8 | reserved | Zero |
| 60 | u32 | unknown60 | Unidentified value |
| 64 | u8 ×2 | reserved | Zero |
| 66 | bool | unknown66 | Unidentified flag |
| 67 | u8 | reserved | Zero |
| 68 | u32 | unknown68 | Unidentified value |
| 72 | u8 ×10 | reserved | Zero |
| 82 | bool | unknown82 | Unidentified flag |
| 83 | u8 | reserved | Zero |
| 84 | u32 | unknown84 | Unidentified value |
| 88 | u8 ×2 | reserved | Zero |
| 90 | u8 | territoryIndex | Joins `territoryinfo.bss` and localization table 12 |
| 91 | u8 | reserved | Zero |
| 92 | u32 | nameStrIdx | Own Korean name in this file's string table |
| 96 | u32 | capitalNameStrIdx | Territory capital's Korean name |
| 100 | u16 | capitalRegionKey | Territory capital; constant within each territory |
| 102 | u16 | affiliatedTownRegionKey | Town responsible for this region |
| 104 | u16 | regionGroupKey | Joins `regiongroupinfo.bss` |
| 106 | u8 | reserved | Zero |
| 107 | u16 | unknown107 | Unidentified relation |
| 109 | u8 ×2 | reserved | Zero |
| 111 | u16 | explorationKey | Associated world-node key from `exploration.bss` |
| 113 | u8 ×2 | reserved | Zero |
| 115 | bool | unknown115 | Unidentified flag |
| 116 | u8 ×3 | reserved | Zero |
| 119 | f32 ×3 | waypointPosition | Position returned by the region waypoint interface |
| 131 | f32 ×3 | position | Region world position |
| 143 | u8 ×4 | reserved | Zero |
| 147 | bool | unknown147 | Unidentified flag |
| 148 | u8 | reserved | Zero |
| 149 | u32 | unknown149 | Unidentified patch-varying value; not a structural marker |
| 153 | f32 ×5 | unknown153 | Unidentified configuration values |
| 173 | u32 | unknown173 | Unidentified value |
| 177 | u32 | unknown177 | Unidentified value |
| 181 | u32 | unknown181 | Observed as `0xffffffff` |
| 185 | u32 ×6 | unknown185 | Unidentified ids, populated mainly on towns |
| 209 | bool | unknown209 | Unidentified flag |

The variable portion immediately follows the head:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u32 | warehouseGroupCount | Number of following region keys |
| 4 | u16 ×n | warehouseGroup | Regions sharing storage/transport topology; includes the owning region |
| 4 + n×2 | u32 | extraPositionCount | Number of following positions |
| 8 + n×2 | f32 ×3 ×m | extraPositions | Additional world-map marks for oversized regions |

The final 171 bytes begin after both lists. Tail offsets below are relative to the start
of that block:

| Tail @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u8 | reserved | Zero |
| 1 | u16 | unknownTail1 | Unidentified value |
| 3 | u16 | unknownTail3 | Unidentified value |
| 5 | f32 ×3 | unknownTail5 | Unidentified vector |
| 17 | u32 | unknownTail17 | Unidentified value |
| 21 | u8 | unknownTail21 | Unidentified value |
| 22 | u8 | unknownTail22 | Unidentified value |
| 23 | u8 | unknownTail23 | Unidentified value |
| 24 | bool | unknownTail24 | Unidentified flag |
| 25 | f32 ×6 | unknownTail25 | Two unidentified vectors |
| 49 | u64 | unknownTail49 | Unidentified value |
| 57 | f32 | unknownTail57 | Unidentified value |
| 61 | f32 | unknownTail61 | Unidentified value |
| 65 | u32 | unknownTail65 | Unidentified value |
| 69 | bool | unknownTail69 | Unidentified flag |
| 70 | bool | unknownTail70 | Unidentified flag |
| 71 | u32 | unknownTail71 | Unidentified value |
| 75 | u16 | unknownTail75 | Unidentified value |
| 77 | u16 | unknownTail77 | Unidentified value |
| 79 | u8 | unknownTail79 | Unidentified value |
| 80 | u8 | reserved | Zero |
| 81 | f32 | unknownTail81 | Unidentified value |
| 85 | u32 ×6 | unknownTail85 | Unidentified values |
| 109 | bool | unknownTail109 | Unidentified flag |
| 110 | f32 ×3 | unknownTail110 | Unidentified vector |
| 122 | f32 ×3 | unknownTail122 | Unidentified vector |
| 134 | u8 | unknownTail134 | Unidentified value |
| 135 | u8 | unknownTail135 | Unidentified value |
| 136 | bool | unknownTail136 | Unidentified flag |
| 137 | u8 | unknownTail137 | Unidentified value |
| 138 | u8 ×7 | reserved | Zero |
| 145 | u8 | unknownTail145 | Unidentified value |
| 146 | u8 ×8 | reserved | Zero |
| 154 | u32 | unknownTail154 | Unidentified value |
| 158 | u32 | unknownTail158 | Unidentified value |
| 162 | u32 | unknownTail162 | Unidentified value |
| 166 | u8 ×3 | reserved | Zero |
| 169 | u16 | guildWharfManagerCharacterKey | Joins guild-wharf service characters such as Robert and Sebastian |

`tables.DecodeRegionInfo` reads these fields sequentially, preserves every typed
unknown, requires all reserved spans to remain zero, validates list and string bounds,
and requires all records to tile the PABR record area exactly. It also verifies that
the capital relation is consistent within each territory.

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
| 4 | u8 | enabled | 1 for active nodes; 0 on seven unused records whose localized name is `UnKnown` |
| 5 | u8 | nodeKind | 16-value `model.WorldNodeKind` enum |
| 6 | u16 | linkedKey | redundant node reference; == `nodeKey` in all 1,037 records |
| 8 | u16 | unknown8 | Usually zero |
| 10 | u16 | nameStrIdx | Korean node name |
| 12 | u8 ×2 | zero | Reserved |
| 14 | u8 | const | 1 |
| 15 | u8 | zero | Reserved |
| 16 | u8 | networkFlag | 1 = normal network node; 0 = special town/sea/district/battlefield location |
| 17 | u8 | unknown17 | Tracks main/sub state except on two Ossuary records and Velia Beach |
| 18 | u8 | mainCopy | Exact copy of the main/sub state at +116 |
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

### `characterfunction.dbss` — NPC services and manager-family ordering

The compact 10-byte offset index is keyed by character-template id. The full variable
record remains only partially mapped. Its prefix describes the NPC's first item-service
module, when one is active:

| @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | u16 | unknownType | Appears to classify the surrounding character-function record |
| 2 | u16 | moduleTag | `0x0600` for the observed item-service prefix |
| 4 | UTF-16 | serviceName | Inline source label such as shop or secret shop; empty means inactive |
| v | UTF-16 | conditionDsl | Client-evaluated access condition; empty means unconditional |
| v | u16 | unknownKey | Trailing module key; meaning unresolved |

This module covers shops, secret shops, exchanges, contracts and similar NPC item
services. Its condition belongs to the service rather than to each item. Consequently,
every item listed for an NPC can inherit the same requirement, such as an exploration
unlock, intimacy threshold, quest completion, class, time window or PC-room check.
Prefix tags `0` and `0x0100` select other character-function layouts and do not contain
this item-service prefix.

The manager join uses a separate exact counted list found later in the record:

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

### `detail_dialog.dbss` — conditioned contribution-point rentals

`detail_dialogoffset.dbss` is a standard 12-byte offset index. Its u32 key packs the
placed dialogue variant in the high word and the character-template id in the low word:

| Bits | Field | Join |
|---:|---|---|
| 0–15 | characterKey | `npcsimply.bss` and `characterfunction.dbss` |
| 16–31 | dialogIndex | A placed NPC variant's `regioninfo` spawn record |

The dialogue record contains several node variants. A normal action node (`variant=0`)
has this variable-width layout:

| Relative @ | Type | Field | Notes |
|---:|---|---|---|
| 0 | UTF-16 | conditionDsl | Client condition; empty means unconditional |
| v | UTF-16 | sourceName | Dialogue-button text |
| v | u16 | unknown0 | Unresolved; zero on all observed rental actions |
| v | u16 | variant | `0` for the action layout decoded here |
| v | UTF-16 | sourceDescription | Dialogue response/description |
| v | UTF-16 | actionDsl | Client action invoked by the button |

Contribution-point rentals use
`buyItemByPoint(itemKey,itemSubKey,count,pointType,pointCost)`. `pointType` is `5` on
all observed rental actions. The paired condition DSL uses semicolon-separated AND
terms and `<or>` between alternative branches; known functions include `getlevel`,
`checkclass`, `getitemcount` and `clearquest`. The extractor keeps the original DSL so
consumers can evaluate or present requirements without losing unknown functions.

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
coincidentally. The "You can learn about X" items (`itemType == 2`, `ItemTypeSkill`) match a theme
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

- **`itemenchant` type-dependent post-icon remainder.** The fixed 18-byte prefix,
  three strings, market limit and following 84-byte prefix are decoded. The preserved
  remainder appears to contain per-slot stat arrays, detailed price/tax rates,
  pet-feed slots, trade/bind/expiry flags and scripts; its variant boundaries are open.
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
