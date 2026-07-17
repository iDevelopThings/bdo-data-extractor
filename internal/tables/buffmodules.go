package tables

import (
	"fmt"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buff.dbss effect-module decoding.
//
// EffectData (92 bytes) is a fixed container, validated over all 44k buff
// records (every nonzero byte falls inside it, and aux is only ever 0 or -1):
//
//	flags [7]u8               @0-6   application flags
//	slots [10]effectSlot      @7-86  the module's arguments
//	tail  [5]u8               @87-91 more application flags
//
//	effectSlot = { value i32; aux i32 }   // aux: 0 or -1 (uncapped marker)
//
// A buff applies ONE effect module: ModuleType selects the kind, and the
// module reads its arguments from the slots by index — like a function call.
// E.g. module 39 (AP up) takes (target, amount) in slots 0,1; module 25
// (EXP gain) takes (amount, kind, lifeSkill) in slots 0,1,2. Percent amounts
// are stored ×10000 (+15% = 150000), flat amounts ×1.
//
// The per-module signatures below are reverse-engineered (fitted against the
// full record set and validated against the live client). Variant names are
// ours — same policy as classBitNames/slotNames: the enum must be hand-
// maintained when PA adds a module, so we keep our own names rather than
// depending on localization at build time. Modules not in the table are the
// conditional on-hit/proc effects and marker buffs; they resolve to nothing
// and the caller falls back to text-based paths.

const (
	effSlotBase   = 7  // first slot's value offset
	effSlotStride = 8  // {i32 value, i32 aux}
	effSlotCount  = 10 // slots in the container
)

// effectArg reads the module's k-th argument (slot k's value).
func effectArg(eff []byte, k int) (int32, bool) {
	off := effSlotBase + effSlotStride*k
	if k < 0 || k >= effSlotCount || len(eff) < off+4 {
		return 0, false
	}
	return int32(bss.U32(eff, off)), true
}

// buffModule is one module's signature: which slot holds the amount, which
// slots select the stat variant, and how the amount is scaled/displayed.
type buffModule struct {
	stat       string  // fixed stat name ("" when paramSlots/variants decide)
	valueSlot  int     // slot index of the amount
	scale      float64 // stored amount = shown amount × scale
	unit       string  // "%" for percent stats
	negate     bool    // stored magnitude represents a reduction (shown "-")
	paramSlots []int   // slot indices forming the variant key
	variants   map[string]string
}

var buffModules = map[byte]buffModule{
	// flat resource stats: (amount) in slot 0. Modules 1-6 are the client's own
	// CppEnums.BuffType_{Current,Max,Regen}{Hp,Mp}VariationAmount — the one place
	// the enum leaks into lua, confirming ModuleType is internally "BuffType".
	2: {stat: "Max HP", valueSlot: 0, scale: 1},
	3: {stat: "HP Recovery", valueSlot: 0, scale: 1},
	5: {stat: "Max MP/WP/SP", valueSlot: 0, scale: 1},
	6: {stat: "MP/WP/SP Recovery", valueSlot: 0, scale: 1},
	8: {stat: "Max Stamina", valueSlot: 0, scale: 1},

	// weight limit: (amount×10000) in slot 0, shown in LT ("+20LT" stored 200000)
	29: {stat: "Weight Limit", valueSlot: 0, scale: 10000, unit: "LT"},

	// percent stats: (amount%) in slot 0
	9:   {stat: "Movement Speed", valueSlot: 0, scale: 10000, unit: "%"},
	10:  {stat: "Attack Speed", valueSlot: 0, scale: 10000, unit: "%"},
	11:  {stat: "Casting Speed", valueSlot: 0, scale: 10000, unit: "%"},
	30:  {stat: "Critical Hit Rate", valueSlot: 0, scale: 10000, unit: "%"},
	47:  {stat: "Horse Capture Rate", valueSlot: 0, scale: 10000, unit: "%"},
	50:  {stat: "Mount EXP", valueSlot: 0, scale: 10000, unit: "%"},
	51:  {stat: "Mount Skill EXP", valueSlot: 0, scale: 10000, unit: "%"},
	56:  {stat: "Amity", valueSlot: 0, scale: 10000, unit: "%"},
	57:  {stat: "Item Drop Rate", valueSlot: 0, scale: 10000, unit: "%"},
	91:  {stat: "Durability Reduction Resistance", valueSlot: 0, scale: 10000, unit: "%"},
	104: {stat: "Karma Recovery", valueSlot: 0, scale: 10000, unit: "%"},
	107: {stat: "Gathering Item Drop Rate", valueSlot: 0, scale: 10000, unit: "%"},
	108: {stat: "Knowledge Gain Chance", valueSlot: 0, scale: 10000, unit: "%"},
	109: {stat: "Higher Grade Knowledge Gain Chance", valueSlot: 0, scale: 10000, unit: "%"},
	112: {stat: "Item Drop Amount", valueSlot: 0, scale: 10000, unit: "%"},
	121: {stat: "Auto-fishing Time", valueSlot: 0, scale: 10000, unit: "%", negate: true},
	129: {stat: "Self-obtainable Black Spirit's Rage", valueSlot: 0, scale: 100, unit: "%"},
	134: {stat: "Swimming Speed", valueSlot: 0, scale: 10000, unit: "%"},

	// flat misc: (amount) in slot 0
	59: {stat: "Jump Height", valueSlot: 0, scale: 1},
	66: {stat: "Energy Recovery", valueSlot: 0, scale: 1},
	94: {stat: "Max Energy", valueSlot: 0, scale: 1},

	// combat stat families: (target, amount) — slot 0 selects
	// 0 melee/base, 1 ranged, 2 magic, 3 all
	39: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]string{
		"0": "Melee AP", "1": "Ranged AP", "2": "Magic AP", "3": "All AP"}},
	40: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]string{
		"0": "Melee Accuracy", "1": "Ranged Accuracy", "2": "Magic Accuracy", "3": "All Accuracy"}},
	41: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]string{
		"0": "Melee Evasion", "1": "Ranged Evasion", "2": "Magic Evasion", "3": "All Evasion"}},
	43: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]string{
		"0": "Melee Damage Reduction", "1": "Ranged Damage Reduction",
		"2": "Magic Damage Reduction", "3": "All Damage Reduction"}},
	46: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]string{
		"0": "Extra AP Against Humans", "1": "Extra AP Against Demihumans",
		"2": "Extra AP Against Beasts", "3": "Extra AP Against Kamasylvian Monsters",
		"4": "Extra AP Against Edanian Monsters"}},

	// crowd-control resistances: (kind, amount%)
	49: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{
		"0": "Knockback/Floating Resistance", "1": "Knockdown/Bound Resistance",
		"2": "Grapple Resistance", "3": "Stun/Stiffness/Freezing Resistance",
		"5": "Stun/Stiffness/Freezing Resistance", "7": "Fear Resistance", "8": "All Resistance"}},
	105: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{
		"0": "Ignore Knockback/Floating Resistance", "1": "Ignore Knockdown/Bound Resistance",
		"2": "Ignore Grapple Resistance", "3": "Ignore Stun/Stiffness/Freezing Resistance",
		"8": "Ignore All Resistance"}},

	// EXP gain: (amount%, kind, lifeSkill) — kind: 0 Combat / 1 Skill / 2 life
	25: {valueSlot: 0, scale: 10000, unit: "%", paramSlots: []int{1, 2}, variants: expVariants()},

	// life-skill EXP, flat item variant: (lifeSkill, amount)
	80: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: lifeSkillVariants(" EXP")},

	// life-skill mastery: (lifeSkill, _, amount)
	149: {valueSlot: 2, scale: 1, paramSlots: []int{0}, variants: masteryVariants()},

	// potential levels: (kind, ranks)
	67: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]string{
		"0": "Movement Speed", "1": "Attack Speed", "2": "Casting Speed",
		"3": "Critical Hit", "4": "Luck", "5": "Fishing Speed", "6": "Gathering Speed"}},

	// special-attack damage: (kind, amount%)
	93: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{
		"0": "All Special Attack Extra Damage", "1": "Back Attack Extra Damage",
		"2": "Down Attack Extra Damage", "3": "Air Attack Extra Damage",
		"4": "Critical Hit Extra Damage", "5": "Speed Attack Extra Damage",
		"6": "Counter Attack Extra Damage"}},

	// weather resistances: (kind, amount%) — kind 0 heat, 1 cold
	128: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{
		"0": "Heatstroke Resistance", "1": "Hypothermia Resistance"}},

	// underwater breathing: (ms) in slot 0, shown in seconds
	95: {stat: "Underwater Breathing", valueSlot: 0, scale: 1000, unit: "sec"},

	// assorted parameterized percent stats: (kind, amount%)
	98:  {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{"1": "Movement Speed"}},
	106: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{"3": "All Damage Reduction"}},
	126: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{
		"1": "Chance to Catch Rare Fish", "2": "Chance to Catch Rare Fish"}},
	181: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]string{
		"0": "Breath EXP", "1": "Strength EXP"}},

	// O'dyllita sun/lunar/earth food buffs: (_, amount, which)
	187: {valueSlot: 1, scale: 100, paramSlots: []int{2}, variants: map[string]string{
		"0": "Sun Buff", "1": "Lunar Buff", "2": "Earth Buff"}},
}

// expVariants builds module 25's variant table: arg1 = 0 Combat / 1 Skill /
// 2 life-skill EXP with arg2 selecting the life skill.
func expVariants() map[string]string {
	v := map[string]string{"0,0": "Combat EXP", "1,0": "Skill EXP"}
	for _, info := range model.LifeSkillTypes.Infos() {
		if info.Playable {
			v[fmt.Sprintf("2,%d", info.Wire())] = info.Title + " EXP"
		}
	}
	v[fmt.Sprintf("2,%d", model.LifeSkillTypeCount)] = "Life EXP"
	return v
}

func lifeSkillVariants(suffix string) map[string]string {
	v := map[string]string{}
	for _, info := range model.LifeSkillTypes.Infos() {
		if info.Playable {
			v[fmt.Sprintf("%d", info.Wire())] = info.Title + suffix
		}
	}
	v[fmt.Sprintf("%d", model.LifeSkillTypeCount)] = "Life" + suffix
	return v
}

func masteryVariants() map[string]string {
	v := map[string]string{}
	for _, info := range model.LifeSkillTypes.Infos() {
		if info.Playable {
			v[fmt.Sprintf("%d", info.Wire())] = info.Title + " Mastery"
		}
	}
	v[fmt.Sprintf("%d", model.LifeSkillTypeCount)] = "Life Skill Mastery"
	return v
}

// customModules handles the modules whose value slot, scale, or unit is chosen
// by a parameter — so the variants can't share one fixed layout. Each returns
// (stat, unit, raw stored value, scale, ok); ok=false means "not a displayable
// effect" (e.g. a module-1 damage-over-time debuff) and the caller moves on.
var customModules = map[byte]func(eff []byte, cond int16) (stat, unit string, raw int32, scale float64, ok bool){
	1:   resolveHpOnHit,      // Recover N HP on Hits / on Critical Hits
	4:   resolveMpOnHit,      // Recover N MP/WP/SP on Hits / on Critical Hits
	111: resolveManufacture,  // Cooking/Alchemy Time (sec) vs Processing Success Rate (%)
	120: resolveMonsterDR,    // Monster Damage Reduction (flat) vs Rate (%)
	136: resolveMonsterExtra, // Extra AP Against Monsters / Adventurers
}

// onHitRecovery: value in slot 0 (>0), nothing else set, ConditionType picks the
// trigger (1 = on hit, 9 = on critical hit). Shared by the HP (module 1) and MP
// (module 4) resource modules. Their other buffs (fixed-damage procs, DoTs,
// periodic drains) are debuffs with negative/multi-slot values, not extracted.
func onHitRecovery(eff []byte, cond int16, base string) (stat, unit string, raw int32, scale float64, ok bool) {
	v0, o0 := effectArg(eff, 0)
	if !o0 || v0 <= 0 {
		return
	}
	for k := 1; k < effSlotCount; k++ {
		if v, _ := effectArg(eff, k); v != 0 {
			return
		}
	}
	switch cond {
	case 1:
		return base + " on Hit", "", v0, 1, true
	case 9:
		return base + " on Critical Hit", "", v0, 1, true
	}
	return
}

func resolveHpOnHit(eff []byte, cond int16) (stat, unit string, raw int32, scale float64, ok bool) {
	if s, u, r, sc, o := onHitRecovery(eff, cond, "HP Recovery"); o {
		return s, u, r, sc, o
	}
	return onHitFixedDamage(eff, cond)
}

// onHitFixedDamage: module 1 with a negative slot-0 value is the offensive twin
// of HP recovery — it deals fixed damage on a trigger rather than healing. The
// amount is shown positive (damage dealt to the target, not a debuff on the
// wearer); ConditionType picks the trigger. cond 0/2 (DoTs, one-off drains) are
// monster/debuff buffs, not item stats, so they fall through unresolved.
func onHitFixedDamage(eff []byte, cond int16) (stat, unit string, raw int32, scale float64, ok bool) {
	v0, o0 := effectArg(eff, 0)
	if !o0 || v0 >= 0 {
		return
	}
	for k := 1; k < effSlotCount; k++ {
		if v, _ := effectArg(eff, k); v != 0 {
			return
		}
	}
	switch cond {
	case 6:
		return "Fixed Damage on Back Attack Hits", "", -v0, 1, true
	case 10:
		return "Fixed Damage on Critical Hits", "", -v0, 1, true
	case 4:
		return "Retaliation Fixed Damage when Struck", "", -v0, 1, true
	}
	return
}

func resolveMpOnHit(eff []byte, cond int16) (stat, unit string, raw int32, scale float64, ok bool) {
	return onHitRecovery(eff, cond, "MP/WP/SP Recovery")
}

// resolveManufacture: value always in slot 1; slot 0 selects the effect and its
// scale/unit — a life-skill time reduction (seconds, shown negative) or the
// processing success-rate percent.
func resolveManufacture(eff []byte, _ int16) (stat, unit string, raw int32, scale float64, ok bool) {
	v1, _ := effectArg(eff, 1)
	if v1 == 0 {
		return
	}
	switch v0, _ := effectArg(eff, 0); v0 {
	case 0:
		return "Alchemy Time", "sec", -v1, 50000, true // reduction -> negative
	case 1:
		return "Cooking Time", "sec", -v1, 50000, true
	case 2:
		return "Processing Success Rate", "%", v1, 10000, true
	}
	return
}

// resolveMonsterDR: value always in slot 1; slot 0 selects flat vs percent rate.
func resolveMonsterDR(eff []byte, _ int16) (stat, unit string, raw int32, scale float64, ok bool) {
	v1, _ := effectArg(eff, 1)
	if v1 == 0 {
		return
	}
	switch v0, _ := effectArg(eff, 0); v0 {
	case 0:
		return "Monster Damage Reduction Rate", "%", v1, 10000, true
	case 2:
		return "Monster Damage Reduction", "", v1, 1, true
	}
	return
}

// resolveMonsterExtra: the value's slot IS the variant — slot 0 = vs Monsters,
// slot 1 = vs Adventurers (both flat).
func resolveMonsterExtra(eff []byte, _ int16) (stat, unit string, raw int32, scale float64, ok bool) {
	if v, _ := effectArg(eff, 0); v != 0 {
		return "Extra AP Against Monsters", "", v, 1, true
	}
	if v, _ := effectArg(eff, 1); v != 0 {
		return "Extra AP Against Adventurers", "", v, 1, true
	}
	return
}

// ResolveBuffStat decodes a buff's effect purely from its module arguments:
// (stat, ±value, unit). ok is false for modules/variants not handled here
// (conditional procs, marker buffs) — callers fall back to the localized-name
// paths for those.
func ResolveBuffStat(b Buff) (stat, op string, val float64, unit string, ok bool) {
	if fn, cok := customModules[b.Module]; cok {
		st, unit, raw, scale, rok := fn(b.EffectData, b.Condition)
		if !rok || raw == 0 {
			return "", "", 0, "", false
		}
		v := float64(raw) / scale
		o := "+"
		if v < 0 {
			o, v = "-", -v
		}
		return st, o, v, unit, true
	}
	m, has := buffModules[b.Module]
	if !has {
		return "", "", 0, "", false
	}
	raw, valid := effectArg(b.EffectData, m.valueSlot)
	if !valid || raw == 0 {
		return "", "", 0, "", false
	}
	stat = m.stat
	if len(m.paramSlots) > 0 {
		keys := make([]string, len(m.paramSlots))
		for i, slot := range m.paramSlots {
			p, pok := effectArg(b.EffectData, slot)
			if !pok {
				return "", "", 0, "", false
			}
			keys[i] = fmt.Sprintf("%d", uint32(p))
		}
		stat = m.variants[strings.Join(keys, ",")]
		if stat == "" {
			return "", "", 0, "", false
		}
	}
	val = float64(raw) / m.scale
	op = "+"
	if m.negate {
		op = "-"
	}
	if val < 0 {
		op, val = "-", -val
	}
	return stat, op, val, m.unit, true
}
