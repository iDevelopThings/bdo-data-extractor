// Package model defines the unified, externally-consumable item record.
package model

import (
	"reflect"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// Item is the merged view of one game item, assembled from several decoded
// client tables. Fields are omitted from JSON when empty/unknown so the output
// stays compact. All values come from the client files (no scraping).
type Item struct {
	*models.BaseFor[Item]

	ID   uint32 `json:"id"`
	Name string `json:"name,omitempty"` // languagedata_<lang>.loc
	// VariantOf groups reissued copies of the same item: the game mints bound
	// reward/season/box variants under new ids (e.g. Nouver Kunai exists 8x:
	// the tradeable base + 7 untradeable copies, some carrying slightly older
	// stat-curve snapshots). Copies matching on (name, itemType, category,
	// grade, icon) point at the canonical record — the tradeable/market item
	// when one exists. 0/absent = canonical. Ghost marks name-only records
	// (a localization entry with no item data behind it — retired/re-id'd
	// items); ghosts are kept for id coverage but carry nothing.
	VariantOf         *models.EntityRef[Item] `json:"variantOf,omitempty"`
	Ghost             bool                    `json:"ghost,omitempty"`
	Description       string                  `json:"description,omitempty"`       // languagedata_<lang>.loc
	UseText           string                  `json:"useText,omitempty"`           // languagedata_<lang>.loc  use/open confirmation; lists box/chest contents
	ExchangeInfo      string                  `json:"exchangeInfo,omitempty"`      // languagedata_<lang>.loc  NPC exchange offers (location + rate)
	Icon              string                  `json:"icon,omitempty"`              // itemenchant.dbss  (embedded path)
	Grade             string                  `json:"grade,omitempty"`             // itemenchant.dbss  GradeType
	Category          string                  `json:"category,omitempty"`          // itemenchant.dbss  ItemClassify (game category)
	ItemType          string                  `json:"itemType,omitempty"`          // itemenchant.dbss  ItemType
	EquipInfo         *EquipInfo              `json:"equipInfo,omitempty"`         // itemenchant.dbss  equip slot/kind/type (equipment)
	MarketCategory    string                  `json:"marketCategory,omitempty"`    // central-market main category (@188, or derived — see Marketable)
	MarketSubCategory string                  `json:"marketSubCategory,omitempty"` // central-market sub category (@189, or derived)
	Marketable        bool                    `json:"marketable,omitempty"`        // the game's central-market-allowed flag (itemenchant iconEnd+0); a market category with marketable=false is derived for unlistable gear (Tuvala/boss gear)
	// BindType is the game's vestedType (binding behavior) — see the BindType
	// constants.
	BindType            BindType      `json:"bindType,omitempty"`
	MarketRegisterLimit int64         `json:"marketRegisterLimit,omitempty"` // max units per market registration (gear 5, potions 500, mem frags 1000); absent = not listable
	Weight              float64       `json:"weight"`                        // itemenchant.dbss  (LT)
	BuyPrice            int64         `json:"buyPrice"`                      // itemenchant.dbss  OriginalPrice
	SellPrice           int64         `json:"sellPrice"`                     // itemenchant.dbss  SellPriceToNpc
	RepairPrice         int64         `json:"repairPrice"`                   // itemenchant.dbss  RepairPrice
	MaxDurability       int           `json:"maxDurability,omitempty"`       // itemenchant.dbss  (u16 @185, equipment)
	Classes             []string      `json:"classes,omitempty"`             // class restriction (itemenchant mask @77); absent = all classes
	ExpirationMinutes   int           `json:"expirationMinutes,omitempty"`   // timed items, e.g. "(7 Days)" = 10080 (u32 @69)
	RequiredLevel       int           `json:"requiredLevel,omitempty"`       // required character level, e.g. 56 for awakening weapons (@97)
	MaxStack            int64         `json:"maxStack,omitempty"`            // explicit stack limit (u32 @101); absent = unlimited/default
	DyeParts            int           `json:"dyeParts,omitempty"`            // dyeable part count (@160)
	CrystalGroup        *CrystalGroup `json:"crystalGroup,omitempty"`        // crystal transfusion group (record footer + loc table 121)
	Gathered            bool          `json:"gathered,omitempty"`            // a raw/gathered material (itemmaking.xml node/collect/fishing)
	// ItemMaterial (@62) groups gear into material/model families: melee &
	// staff weapons 40, longbow 42, kunai 51, classic boss armor 71 vs other
	// armor 72, accessories by slot (necklace 73, ring 74, earring 75,
	// belt 76), horse gear 123. Named by reference-dictionary correlation
	// (99% match; previously misattributed to NewEquipType — see FORMATS §3).
	ItemMaterial int `json:"itemMaterial,omitempty"`

	// The following were identified by correlating itemenchant rows against a
	// labeled reference field dictionary over the ~28k shared item ids
	// (two-sided agreement ≥93% unless noted; FORMATS §3).
	Stackable     bool  `json:"stackable,omitempty"`     // @67 IsStack
	ApplyDirectly bool  `json:"applyDirectly,omitempty"` // @68 DoApplyDirectly — consumed/applied on obtain
	VestedType    int   `json:"vestedType,omitempty"`    // @73 — the classic binding enum; distinct from BindType (icon+15), which the tooltip/bind-warning UI reads
	UserVested    bool  `json:"userVested,omitempty"`    // @74 IsUserVested — character/family-bound
	ForTrade      bool  `json:"forTrade,omitempty"`      // @75 IsForTrade — a trading-skill goods item (crates, trade packs)
	TradeType     int   `json:"tradeType,omitempty"`     // @76 — trade-goods subtype
	LifeExpType   int   `json:"lifeExpType,omitempty"`   // @105 — life-skill XP family
	EventType     int   `json:"eventType,omitempty"`     // @134 ContentsEventType (u8; 171/0 = none, omitted)
	EventParam1   int64 `json:"eventParam1,omitempty"`   // @136 ContentsEventParam1 (u32; often a content ref id)
	EventParam2   int64 `json:"eventParam2,omitempty"`   // @140 ContentsEventParam2 (u32)
	ShownInNote   bool  `json:"shownInNote,omitempty"`   // @151 == 0 (inverse of HideFromNote; most items are hidden from the note UI)
	Cash          bool  `json:"cash,omitempty"`          // @152 IsCash — pearl-shop item
	CronEnchant   int   `json:"cronEnchant,omitempty"`   // @153 CronEnchantcontrol
	Dyeable       bool  `json:"dyeable,omitempty"`       // @156 IsDyeable (DyeParts @160 holds the part count)
	PersonalTrade bool  `json:"personalTrade,omitempty"` // @184 IsPersonalTrade — can be handed player-to-player (Beer, Purified Water, Star Anise Tea…; 329 items)
	NodeFreeTrade bool  `json:"nodeFreeTrade,omitempty"` // @190 — trade goods sellable without node connection
	// FamilyInventory is the client's checkPushFamilyInventory flag (icon-block
	// +13 == 2): the item can be stored in the account-wide Family Inventory to
	// share across your characters. Family Inventory is a restricted whitelist
	// (~1,089 items — mostly potions, cooked food, pearl goods, timed EXP/buff
	// scrolls like Book of Combat/Life and Mercenary's Life); most items default
	// to not-allowed.
	FamilyInventory bool `json:"familyInventory,omitempty"`
	// ContributionCost is the Contribution Points required to rent or place the
	// item (the reference schema's NeedContribute), recovered when you return
	// it. Set on 233 items: "[CP]" rental gear (e.g. [CP] Nesser Longsword = 50,
	// [CP] Kaia/Carnage weapons) and placeable fences/tents ([CP] Strong Fence
	// = 10). Decoded from the type-dependent tail (a u32 after a marker); 0 = none.
	ContributionCost int `json:"contributionCost,omitempty"`

	// Still-unidentified item-row bytes, read into neutral typed fields (embedded
	// so each promotes to a top-level JSON key). See ItemUnknowns.
	ItemUnknowns

	// Acquisition, from the per-item info XML (ui_html/xml/<lang>/<id>.xml):
	Vendors      []string `json:"vendors,omitempty"`      // NPCs that sell it (<shop>)
	GatheredFrom []string `json:"gatheredFrom,omitempty"` // gather/collect sources, e.g. "Wild Flax" (<collect>)
	GatherNodes  []string `json:"gatherNodes,omitempty"`  // gathering node regions (<node region="…">)

	// Consumable (food/elixir) buff info, decoded from the item->skill->buff
	// chain (skill.dbss + buff.dbss), named via loc table 5. Fully client-typed.
	Effects *Effects `json:"effects,omitempty"`

	MaxEnhance  *int         `json:"maxEnhance,omitempty"`  // itemmaxlevel.dbss
	Enhancement *Enhancement `json:"enhancement,omitempty"` // enchantstaticstatus.dbss curve
}

func (i *Item) GetMaxEnhancement() *EnchantLevel {
	if i.Enhancement == nil || len(i.Enhancement.Levels) == 0 {
		return nil
	}
	return new(i.Enhancement.Levels[len(i.Enhancement.Levels)-1])
}

// FindEnchantLevel resolves the EnchantLevel for level, clamped to the
// enhancement's range — mirroring the frontend slider's clamp behavior
// (DetailStore.setLevel) so an out-of-range level still resolves sensibly.
func (i *Item) FindEnchantLevel(level int) *EnchantLevel {
	if i.Enhancement == nil || len(i.Enhancement.Levels) == 0 {
		return nil
	}

	minL := i.Enhancement.Levels[0].Level
	maxL := i.Enhancement.Levels[len(i.Enhancement.Levels)-1].Level
	if level < minL {
		level = minL
	}
	if level > maxL {
		level = maxL
	}

	for l := range i.Enhancement.Levels {
		if i.Enhancement.Levels[l].Level == level {
			return &i.Enhancement.Levels[l]
		}
	}

	return nil
}

func (i *Item) GetDurability(enchant *EnchantLevel) int {
	dur := i.MaxDurability
	if enchant != nil && enchant.Durability > 0 {
		dur = enchant.Durability
	}
	return dur
}

// ItemUnknowns are the profiled-but-unidentified item-row bytes, each read into
// a neutral typed field so the schema stays visible and renameable — when a
// field's meaning is confirmed, rename it here and every consumer's key updates
// with it. It is embedded in Item so each field promotes to a TOP-LEVEL JSON key
// (unknown144, unknownIcon19, …). Naming: unknown<off> = the absolute header
// byte offset; unknownIcon<off> = offset from the end of the icon string (a
// different region, so the two number spaces overlap and must be distinguished).
// Every field is deviation-only (a *int, omitempty): nil means the byte holds
// its dominant default, so presence marks the item as unusual for that field —
// the raw material for mapping the rest of the schema. Wide-distribution data
// blocks (header @158-159/@164-165, icon +17..58) are read but deliberately not
// surfaced here — inspect their raw bytes with `seqtail`. Defaults + value
// distributions: FORMATS §3.
type ItemUnknowns struct {
	// header @8-13 — a cluster of boolean/enum flags (unknown11 is set on 26,406
	// items; unknown12 defaults to 2).
	Unknown8  *int `json:"unknown8,omitempty"`
	Unknown9  *int `json:"unknown9,omitempty"`
	Unknown10 *int `json:"unknown10,omitempty"`
	Unknown11 *int `json:"unknown11,omitempty"`
	Unknown12 *int `json:"unknown12,omitempty"`
	Unknown13 *int `json:"unknown13,omitempty"`
	// header @85-88 — a single u32 bitfield of item property flags (set on 903
	// items, 29 distinct bit combos; 0x1ec28000 dominant on 287). Read as one u32,
	// not four bytes.
	Unknown85 *int `json:"unknown85,omitempty"`
	// header @93 — small enum (1|4); equals unknown98 on every one of the 105
	// items that set it (a duplicated field).
	Unknown93 *int `json:"unknown93,omitempty"`
	// header @98-100, @106-108 — small enums on ~100 items each.
	Unknown98  *int `json:"unknown98,omitempty"`
	Unknown99  *int `json:"unknown99,omitempty"`
	Unknown100 *int `json:"unknown100,omitempty"`
	Unknown106 *int `json:"unknown106,omitempty"`
	Unknown107 *int `json:"unknown107,omitempty"`
	Unknown108 *int `json:"unknown108,omitempty"`
	// header @135 — reads 171 (the ContentsEventType "no-event" sentinel) on 104
	// items; likely part of the event struct at @134.
	Unknown135 *int `json:"unknown135,omitempty"`
	// header @144 (u16) — a per-item-class constant on ~381 Etc/material items
	// (trade loot 1000, fences 10, reforge stones 149); NOT the CP cost.
	Unknown144 *int `json:"unknown144,omitempty"`
	// header @146-150 — @148 small enum; @149/@150 default 255.
	Unknown146 *int `json:"unknown146,omitempty"`
	Unknown148 *int `json:"unknown148,omitempty"`
	Unknown149 *int `json:"unknown149,omitempty"`
	Unknown150 *int `json:"unknown150,omitempty"`
	// header @154-155 (@155 weakly matched isGuildStockable, 74%), @157 default 1.
	Unknown154 *int `json:"unknown154,omitempty"`
	Unknown155 *int `json:"unknown155,omitempty"`
	Unknown157 *int `json:"unknown157,omitempty"`
	// header @161, @168-169, @176-178. unknown168 + unknown176 are a PAIRED field
	// — they fire on the exact same 16,655 items (8 bytes apart, ~20 values each),
	// a strong candidate for a two-part record.
	Unknown161 *int `json:"unknown161,omitempty"`
	Unknown168 *int `json:"unknown168,omitempty"`
	Unknown169 *int `json:"unknown169,omitempty"`
	Unknown176 *int `json:"unknown176,omitempty"`
	Unknown177 *int `json:"unknown177,omitempty"`
	Unknown178 *int `json:"unknown178,omitempty"`
	// header @187, @191 (default 1) — flags.
	Unknown187 *int `json:"unknown187,omitempty"`
	Unknown191 *int `json:"unknown191,omitempty"`

	// icon-block +1..+3 — small enums; unknownIcon2 defaults to 9 (its only
	// deviants are the currencies: Silver/Loyalties/Crow Coin/Hardcore Coin).
	UnknownIcon1 *int `json:"unknownIcon1,omitempty"`
	UnknownIcon2 *int `json:"unknownIcon2,omitempty"`
	UnknownIcon3 *int `json:"unknownIcon3,omitempty"`
	// icon-block +8 (default 2): value 1 on 271 Cook items — both finished dishes
	// and effectless intermediate sauces/ingredients. Likely a cooking-product/
	// ingredient flag. NOT a food training-stat type (it's on effectless sauces
	// and doesn't track the dish's buff).
	UnknownIcon8 *int `json:"unknownIcon8,omitempty"`
	UnknownIcon9 *int `json:"unknownIcon9,omitempty"`
	// icon-block +10 (default 0): value 15/85 on 52 finished foods. Likely a food
	// grade/tier or buff-duration bracket — constant (15) across dishes with very
	// different buffs, so NOT a per-stat training amount.
	UnknownIcon10 *int `json:"unknownIcon10,omitempty"`
	UnknownIcon14 *int `json:"unknownIcon14,omitempty"`
	// icon-block +16 (default 1): a ternary. 2 = a raw gatherable/worker-node
	// resource (ore, timber, fish, hide, crops — Iron Ore, Coal, Rough Stone…);
	// 0 = special/pearl/cash goods (SpecialGoods, PearlGoods). Best guess: an
	// item resource/source class.
	UnknownIcon16 *int `json:"unknownIcon16,omitempty"`
	UnknownIcon19 *int `json:"unknownIcon19,omitempty"`
	UnknownIcon27 *int `json:"unknownIcon27,omitempty"`
}

// Map returns the populated unknown fields keyed by their JSON name (unknown144,
// unknownIcon19, …); nil (default-valued) fields are omitted. It reflects over
// the struct's json tags, so renaming a field automatically updates the key —
// a convenience for generic tooling (e.g. the viewer's Unknowns Explorer) that
// iterates the unknowns without hard-coding the field list.
func (u ItemUnknowns) Map() map[string]int64 {
	t := reflect.TypeOf(u)
	v := reflect.ValueOf(u)
	out := make(map[string]int64, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Ptr && !f.IsNil() {
			key, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			out[key] = f.Elem().Int()
		}
	}
	return out
}

func (i *Item) HasEffect(q string) bool {
	if i.Effects == nil {
		return false
	}
	for _, group := range [][]StatMod{i.Effects.Stats.Stats, i.Effects.Hidden.Stats} {
		for _, s := range group {
			if strings.Contains(strings.ToLower(s.Stat), q) {
				return true
			}
		}
	}

	return false
}

// BindType is the game's item binding behavior ("vestedType" internally — the
// tooltip's bind label and the "will become bound" use-warning read it).
type BindType byte

const (
	BindNone      BindType = 0 // never binds (boss gear stays market-listable after equipping)
	BindOnObtain  BindType = 1 // binds when obtained (season gear)
	BindOnEquip   BindType = 2 // binds when equipped/used (Manos, Loggia, Tuvala)
	BindOnObtain2 BindType = 3 // obtain-bound variant (crow/abyss gear)
)

func (b BindType) String() string {
	switch b {
	case BindNone:
		return "none"
	case BindOnObtain:
		return "bindsOnObtain"
	case BindOnEquip:
		return "bindsOnEquip"
	case BindOnObtain2:
		return "bindsOnObtain2"
	}
	return "unknown"
}

// EquipInfo groups an equippable item's slot taxonomy.
type EquipInfo struct {
	// Slot is the normalized, class-independent
	// equip slot (@14) — the only slot source for artifacts, life/gathering tools and
	// costume accessories whose Type is blank;
	Slot string `json:"slot,omitempty"`
	// Kind is the broad gear class (Weapon/Armor/Other, itemenchant @15);
	Kind string `json:"kind,omitempty"`
	// Type is the specific EquipType (@7), the weapon class for weapons.
	Type string `json:"type,omitempty"`
	// Slots lists every slot the item occupies (@14 + @16-18)
	// for multi-slot items like functional costumes; absent = single-slot.
	Slots []string `json:"slots,omitempty"`
}

// CrystalGroup is a socket crystal's transfusion group: at most Max crystals of
// the same group can be transfused at once (Max 1000 = no limit).
type CrystalGroup struct {
	Name string `json:"name"`
	Max  int    `json:"max"`
}

// Effects is what a consumable applies on use: a shared cooldown/duration, the
// shown stat modifiers, and any hidden buffs the client doesn't display (kept
// separate; their names may still be untranslated Korean pending a future map).
type Effects struct {
	CooldownMs int         `json:"cooldownMs,omitempty"` // item-use cooldown (skill.dbss)
	DurationMs int         `json:"durationMs,omitempty"` // buff duration (shared)
	Stats      EffectGroup `json:"stats,omitempty"`
	Hidden     EffectGroup `json:"hidden,omitempty"`
}

// EnchantLevel is the gear stats + effects at one enhancement level.
//
// Caphras is the Caphras chart for this level — the 20 Caphras step stats
// stacked on top of the level's base stats. Only levels 18/19/20 (TRI/TET/PEN)
// of a Caphras-enhanceable item carry it; nil otherwise. It lives here (rather
// than as a parallel per-level array on Enhancement) because a Caphras chart is
// inherently the stats added at a given enhancement level.
type EnchantLevel struct {
	Level int `json:"level"`
	// Name is the scheme-aware display label — "Base", "+1"…"+15", or a roman tier
	// "PRI (I)"…"DEC (X)" (accessories/endgame lines start at PRI, gear runs +1..+15
	// first). Precomputed so consumers don't re-derive the scheme.
	Name            string `json:"name"`
	ApMin           int    `json:"apMin,omitempty"`
	ApMax           int    `json:"apMax,omitempty"`
	Ap              int    `json:"ap,omitempty"`       // display AP = round((min+max)/2)
	Accuracy        int    `json:"accuracy,omitempty"` // weapon accuracy (post-DSL tail; hidden in the tooltip for most weapons)
	Evasion         int    `json:"evasion,omitempty"`
	DamageReduction int    `json:"damageReduction,omitempty"`
	Dp              int    `json:"dp,omitempty"` // = evasion + damageReduction
	// AddedEvasion / AddedDamageReduction are the extra defense a sub-weapon/armor
	// piece grants on top of its own Evasion/DamageReduction (the "(+N)" in the
	// tooltip, e.g. Blackstar Horn Bow "Evasion 11 (+33)"). From the post-DSL tail.
	AddedEvasion         int `json:"addedEvasion,omitempty"`
	AddedDamageReduction int `json:"addedDamageReduction,omitempty"`
	MaxHP                int `json:"maxHp,omitempty"`
	// Durability is the item's max durability at this enhancement level — it rises
	// with enhancement (base 100 through TRI 160 / TET 180 / PEN 200), so it's a
	// real per-level value, not constant.
	Durability int `json:"durability,omitempty"`
	// EnhanceChance is the base success probability (0 failstacks) to enhance at this
	// level — a fraction 0..1 (@41 ÷ 1,000,000). Follows the game's curve: 1.0 for
	// +1..+7, then dropping (PRI ~0.13, TET ~0.005, PEN ~0.002).
	EnhanceChance float64        `json:"enhanceChance,omitempty"`
	Effects       []EffectGroup  `json:"effects,omitempty"` // item + set effects (DSL formulas)
	Caphras       []CaphrasLevel `json:"caphras,omitempty"` // Caphras steps at this level (18/19/20 only)

	// Every remaining field of the record, read in sequence so nothing is skipped.
	EnchantUnknowns
}

func (e *EnchantLevel) GetApRange(bonus float64) (ap, apMin, apMax float64, isRange bool) {
	ap = float64(e.Ap) + bonus
	isRange = e.ApMax > 0 && e.ApMin != e.ApMax
	if isRange {
		apMin = float64(e.ApMin) + bonus
		apMax = float64(e.ApMax) + bonus
	} else {
		apMin = 0
		apMax = 0
	}
	return
}
func (e *EnchantLevel) GetCaphrasLevel(level int) *CaphrasLevel {
	for _, s := range e.Caphras {
		if s.Level != level {
			continue
		}
		return &s
	}
	return nil
}

// EnchantUnknowns are the still-unidentified scalar fields of an enchant-curve
// record. They're read in the same sequential pass as the named stats (never
// seeked past), so the whole record is consumed and nothing is silently dropped.
// Each is a *int that stays nil (omitted from JSON) when zero, like ItemUnknowns —
// rename one to a real stat once identified.
type EnchantUnknowns struct {
	Unknown4     *int `json:"unknown4,omitempty"`     // @4  u32 per-record value (hash/id-like)
	Unknown8     *int `json:"unknown8,omitempty"`     // @8  u32
	Unknown25    *int `json:"unknown25,omitempty"`    // @25 u32 (rises with enhancement — likely material/cron cost)
	Unknown45    *int `json:"unknown45,omitempty"`    // @45 u32 (often 1,000,000 — rate-like)
	Unknown55    *int `json:"unknown55,omitempty"`    // @55 u16 (10, steps to 20 at PRI — enhancement param)
	Unknown57    *int `json:"unknown57,omitempty"`    // @57 u16 (constant 10)
	Unknown60    *int `json:"unknown60,omitempty"`    // @60 u16 (rises +8→+15 then caps at 100 — enhancement param)
	Unknown70    *int `json:"unknown70,omitempty"`    // @70 f32 — the one populated slot of the @66-165 per-species-AP band (5-10 on ~336 records, specific gear at TRI/PEN)
	Unknown167   *int `json:"unknown167,omitempty"`   // @167 tri-block (tracks minAP-1)
	UnknownRate1 *int `json:"unknownRate1,omitempty"` // post-DSL constant (usually 1,000,000)
	UnknownRate2 *int `json:"unknownRate2,omitempty"` // post-DSL constant (usually 700,000)
}

// Enhancement is an item's full per-level curve, linked via its EnchantKey (baseId).
//
// CaphrasCategory keys the Caphras chart (caphras.json) the item follows at
// enhancement 18-20; 0 = not Caphras-enhanceable. The game computes this key
// inside the client executable, so it is DERIVED here from client-side fields
// (slot kind x a buy-price tier: boss >= 10M, blue >= 2M, else green) --
// validated chart-exact against 1,315 community-labeled items with no false
// positives in the eligibility domain (max level 20 weapons/armor).
type Enhancement struct {
	*models.BaseFor[Enhancement]

	BaseID          uint32                             `json:"baseId"`
	CaphrasCategory *models.EntityRef[CaphrasCategory] `json:"caphrasCategory,omitempty"`
	// Levels is the per-enhancement-level curve; the Caphras chart (for
	// Caphras-enhanceable items) is embedded on the 18/19/20 EnchantLevels
	// themselves (EnchantLevel.Caphras), so consumers need no join.

	MinLevel    int `json:"minLevel"`    // the lowest level in Levels (usually 0, sometimes 1)
	MinLevelIdx int `json:"minLevelIdx"` // the index of the EnchantLevel matching MinLevel

	MaxLevel    int `json:"maxLevel"`    // the highest level in Levels (usually 15, sometimes 20)
	MaxLevelIdx int `json:"maxLevelIdx"` // the index of the EnchantLevel matching MaxLevel

	Levels []EnchantLevel `json:"levels,omitempty"`
}

// Ingredient is one material input of a recipe (item id + quantity).
// Count is 0 when unknown (the per-item recipe XMLs list ingredients but not amounts).
type Ingredient struct {
	Item  *models.EntityRef[Item] `json:"item"`
	Count int                     `json:"count,omitempty"`
}

// Recipe is one crafting recipe, decoded from the per-item recipe XMLs
// (ui_html/xml/<id>.xml). Output is the produced item; Type is COOK/ALCHEMY, the
// manufacture action (HEAT/GRIND/DRY/…), or HOUSE for worker-building crafting.
// Station is the workshop name for HOUSE recipes (e.g. "Jeweler"). An item often
// has several recipes (alternative ingredient sets).
type Recipe struct {
	*models.BaseFor[Recipe]

	Output  *models.EntityRef[Item] `json:"output"`
	Type    string                  `json:"type"`
	Station string                  `json:"station,omitempty"`
	Inputs  []Ingredient            `json:"inputs"`
	// ByproductOf is set when this recipe does not actually craft Output — Output
	// procs from it as a low-chance byproduct while the recipe really produces
	// ByproductOf (one of Output's own ingredients). 0 = a normal recipe. Detected
	// structurally: the recipe is identical to the recipe of one of Output's
	// ingredients, so crafting that ingredient is what yields Output. Consumers
	// should exclude byproduct recipes from the craft tree but may still show
	// "also obtainable as a byproduct of <ByproductOf>".
	ByproductOf *models.EntityRef[Item] `json:"byproductOf,omitempty"`
}
