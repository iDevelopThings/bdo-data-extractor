# bdo-data-extractor

A single, fast, read-only Go CLI that decodes **Black Desert Online's** client
data files — the `PAZ` archives, the `.bss`/`.dbss` binary tables, the per-item
recipe XMLs, and the `.loc` localization — into clean cached JSON (`items.json`,
`recipes.json`) plus item icons (PNG).

It also decodes NPCs, grind spots, regions/nodes, fishing spots, and the full
**knowledge/ecology** tree.

Everything comes from **your own installed game files**. There is no scraping and
no dependency on third-party data sites — the point of this project is that the
data is *in your client*, and you shouldn't need to be gated behind someone
else's website to get at it.

The **one exception** is live **Central Market prices**: the market economy is
server-side and live, so it genuinely isn't in the client. The extracted reference
data stays 100% client-sourced, and the extractor makes no network calls at all.

```jsonc
// one entry from items.json
{
  "id": 9359,
  "name": "Balacs Lunchbox",
  "icon": "New_Icon/03_ETC/07_ProductMaterial/00009359.dds",
  "grade": "yellow",
  "category": "Cook",
  "marketCategory": "Consumables",
  "marketSubCategory": "Food",
  "weight": 0.1,
  "buyPrice": 38775,
  "sellPrice": 1551,
  "effects": {
    "cooldownMs": 1800000,
    "durationMs": 5400000,
    "stats": [
      { "stat": "Fishing EXP", "op": "+", "value": 10, "unit": "%", "buff": 55948 },
      { "stat": "Auto-fishing Time", "op": "-", "value": 7, "unit": "%", "buff": 55942 },
      { "stat": "Fishing", "op": "+", "value": 2, "buff": 50675 }
    ],
    "hidden": [ { "stat": "Health EXP", "op": "+", "value": 100, "buff": 50562 } ]
  }
}
```

## How this was made

The binary formats decoded here had **no public documentation** — community sites
expose finished data but not how to read the files yourself. This whole project,
including the reverse-engineering, was done almost entirely by **Claude**
(Anthropic's AI) over a series of sessions, then **verified by the maintainer**:
it builds, it runs, and the decoded values are cross-checked against the live
game and community references for correctness.

The format notes are written down in **[FORMATS.md](FORMATS.md)** on purpose —
so the next person who needs this doesn't have to start from nothing. Take it,
learn from it, extend it.

## Disclaimer

> This is an **independent, unofficial** tool. It is **not affiliated with,
> endorsed by, or connected to Pearl Abyss Corp.** "Black Desert Online" and all
> related names and marks are trademarks of Pearl Abyss, used here only to
> identify the game these files belong to.
>
> **No game code, assets, or data are included or redistributed** in this
> repository. `bdo-data-extractor` only *reads* files from a copy of Black Desert Online
> that **you have legally installed on your own machine** — you must own and
> install the game yourself. It is strictly **read-only** on the game directory
> (writing inside it is refused in code), and it redistributes nothing from it.
>
> The format documentation is the result of **independent reverse-engineering for
> interoperability and educational purposes** — understanding the structure of
> data you already own. It is provided for personal, educational, and
> non-commercial use (see [LICENSE](LICENSE.md)). Respect the game's Terms of
> Service. Use at your own risk; no warranty.
>
> **A note to Pearl Abyss:** if you'd rather tools like this didn't have to exist,
> the fix is genuinely easy — ship a small `items.json`/CSV alongside the game,
> or expose a simple read-only API for the basic reference data (items, recipes,
> nodes, drops). Give the community access to the simple things and nobody needs
> to reverse-engineer the client to build a fan tool. It doesn't have to be clean
> or well-structured — even a messy, awkward dump would be far better than nothing,
> and it would save everyone (us *and* you) a lot of trouble.
>
> If you are Pearl Abyss and would like something changed or removed, please open
> an issue and it will be addressed.

## Requirements

- **Go 1.26+** (the project tracks the latest Go). The CLI is **pure stdlib** — no
  external dependencies.
- A **legally installed copy of Black Desert Online**. By default the tool looks
  in the standard Steam path
  (`C:\Program Files (x86)\Steam\steamapps\common\Black Desert Online`); override
  with `--game <dir>`. Default paths are Windows; pass `--game` elsewhere.
- **Windows** is the supported target (it's where BDO runs); other platforms build
  from source — PRs to support them are welcome.

## Build & run

```sh
go install github.com/idevelopthings/bdo-data-extractor@latest   # or, from source: go build -o bdo-data-extractor .

# decode everything -> ./data/items.json + ./data/recipes.json
bdo-data-extractor build

# other commands
bdo-data-extractor icons                       # decode item icons (DXT .dds) -> ./data/icons/<id>.png (+ zone-category icons -> ./data/icons/zonecategories/)
bdo-data-extractor knowledge-icons             # decode knowledge card images -> ./data/knowledge_icons/<path>.png
bdo-data-extractor regionmaps                  # decode the region masks -> ./data/regionmaps/*.png
bdo-data-extractor loc                         # dump the entire localization -> ./data/loc_<lang>.json
bdo-data-extractor meta                        # parse the archive index, print a summary
bdo-data-extractor extract <substr> <outDir>   # extract decoded archive files whose path contains substr
bdo-data-extractor table <name>                # decode one schema-known table -> JSON (stdout)
```

Flags (any position): `--game <dir>` `--out <dir>` `--lang <en|de|fr|sp>`
`--pretty` (indent JSON).

A full `build` reads each source table once and finishes in a few seconds for
~72,000 items. Output is compact and deterministic (sorted by id).

## Output

| File | Contents |
|---|---|
| `data/items.json` | every item: name, description, icon path, grade, category, **market category/sub** (real when market-listed, else *derived* for untradeable gear — see `tradeable`), **`equipInfo`** (slot / kind / type, plus `slots` for multi-slot costumes; `type` = the market-style item type, present even for untradeable items like Tuvala), weight, buy/sell/repair prices, **max durability**, **class restriction** (`classes` — absent = all classes), **`crystalGroup`** (transfusion group + max count for socket crystals), **`expirationMinutes`** (timed items), **`requiredLevel`**, **`maxStack`**, **`dyeParts`**, max enhance, enhancement curve, consumable effects, and acquisition (`vendors` that sell it, `gatheredFrom`/`gatherNodes`) |
| `data/recipes.json` | crafting recipes `{output, type, station, inputs:[{item,count}]}` — cooking/alchemy/processing **and House Crafting** (`station` = the workshop, e.g. "Jeweler"); merged from both the localized and base per-item XMLs |
| `data/marketcategories.json` | the Central Market category tree `[{id, name, subCategories:[{id, name}]}]` in the game's display order (mains by id, subs by sub-id); the `id`/sub-`id` are the same values items carry as `marketCategory`/`marketSubCategory` |
| `data/knowledge.json` | the full knowledge/ecology dataset — `themes` (the category tree, `{key,name,parent,item}`) + `entries` (cards, `{key,theme,name,description,image,minFavor,maxFavor,interest,item,character}`); linked to items and NPCs by name |
| `data/mastery.json` | life-skill **mastery proc/yield curves** keyed by mastery value — `cooking`/`alchemy` (rate columns) + `processing` (`procRate`, mass-process `batch`). These are the client-side proc rates the game applies on top of a base output of 1; the per-recipe yield range itself is server-side |
| `data/manufacture.json` | manufacture/processing recipes from `manufacture.bss` — `{group, type, success, inputs}` (success rate + action type; the output is the `group`/ResultDropGroup, resolved server-side, so it isn't listed) |
| `data/npcs.json` | NPCs `{id, name, title, spawns:[{region, regionName, pos}]}` — **English** names (loc table 6), with each spawn's town/area name (loc table 17) |
| `data/regions.json` | regions/nodes `{key, bounds:{min,max}, spawns:[{key, pos, dialogIndex}]}` — every NPC/monster placement + world position, with region world-bounds where available |
| `data/fishingspots.json` | float-fishing spot world locations (client-side; the fish-per-spot tables are server-side), attributed to regions |
| `data/worldmap.json` | the region-map base image + the world→image transform, so any world-coordinate data can be placed on the map |
| `data/zones.json` | all **105** Monster Zone Info zones, fully self-contained — `name`, `node:{key,name,pos}`, `mainCategory:{id,name,icon}` (region) + `subCategories:[{id,name,icon}]` (content filters), recommended `sheet`/`total` AP/DP + `effectiveLimit`/`apApplyPercent`, and inline resolved lists: `titles[{id,name,desc}]`, `recurringQuests`/`regionQuests[{id,name}]`, `ecology[{id,name}]` (creature knowledge), `topography[{id,name}]` (place knowledge), `tags[{key,name,desc,color,fontColor}]`, plus `loot` as item ids (→ items.json) |
| `data/icons/<id>.png` | item icons, decoded from the client's DXT-compressed `.dds` (via `bdo-data-extractor icons`) |
| `data/knowledge_icons/<path>.png` | knowledge card images, decoded from `.dds` (via `bdo-data-extractor knowledge-icons`); each entry's `image` field is the relative path |
| `data/regionmaps/*.png` | decoded region/territory mask images (via `bdo-data-extractor regionmaps`) |
| `data/icons/zonecategories/<iconId>.png` | Monster Zone Info main/sub-category icons, cropped from their UI atlas (the `icon` ids in `zones.json`), in all three UI states: `<iconId>.png` (normal), `<iconId>_Over.png`, `<iconId>_Click.png`; produced by `bdo-data-extractor icons` |
| `data/loc_<lang>.json` | the full localization dump, grouped `table → id → field → text` (via `bdo-data-extractor loc`) |

Icons are named by **item id** (the client stores them under unrelated icon ids),
so they map straight back to `items.json`.

## Architecture

```
main.go                    CLI entry point + subcommands (build, icons, loc, meta, table, …)
pipeline/                  icon / region-map / knowledge-icon / loc-dump pipelines
internal/
  config/   flag parsing + global config
  paz/      archive layer: ICE cipher, BDO-LZ, meta index, read-only access
  bss/      .bss/.dbss record reader + schema + offset-index + PABR helpers
  loc/      languagedata_<lang>.loc decoder
  tables/   the leaf parsers: items, enchant curves, buffs/skills, recipe XML, npcs,
            regions, zones, fishing, region maps, knowledge (mentalcard/mentaltheme)
  build/    the assembler: Builder reads sources, joins across tables, writes JSON
            (items, recipes, market, world, zones, fishing, knowledge, mastery, caphras)
  schema/   declarative table schemas (used by `table`)
  jsonio/   buffered + parallel JSON file writer
  tex/      DXT1/DXT5/uncompressed DDS -> RGBA
  progress/ pipeline progress sink
src/
  model/    the unified output structs
  models/   base/ref helpers for the output types
  recipe/   recipe-classification helpers (extraction / byproduct / imperial)
  urn/      stable urn identifiers for output records
  utils/    small generic helpers (timing, slices, strings)
```

See **[FORMATS.md](FORMATS.md)** for the reverse-engineered file formats and
table layouts.

## Contributing / help wanted

This was reversed from scratch with no reference, so plenty is still unmapped
(see **[FORMATS.md → Unmapped & contributing](FORMATS.md#15-unmapped--contributing)**).
If you have **extra schema, headers, struct/field layouts, or any information** that
helps decode these files, please **open a PR** — or just **open an issue and share what
you know**. Even partial notes let us figure out the rest and update the tool. The whole
point is to make this knowledge available to the community rather than gatekeep it.

Especially wanted right now:
- the `itemenchant` post-icon tail (~185 columns) and the gear pre-name block,
- the `buff.dbss` effect-module names (the ~166 not in any client table),
- the `dropuihuntinggroundinfo.bss` per-zone record boundaries (the *loot lists* are
  decoded in `zones.json`; attributing each item to its exact zone isn't finished),
- per-monster **drop rates** and **combat stats** (these appear to be server-side).

## License

[Non-commercial license](LICENSE.md) — free to use, study, modify, and share for
any **non-commercial** purpose; you may not sell it or use it to make money. It
covers only this project's source code, makes no claim over Black Desert Online
or Pearl Abyss's data/assets, and is provided **as-is, with no warranty**.
