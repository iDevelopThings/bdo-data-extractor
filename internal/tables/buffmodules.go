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
func effectArg(eff [92]byte, k int) (int32, bool) {
	off := effSlotBase + effSlotStride*k
	if k < 0 || k >= effSlotCount {
		return 0, false
	}
	return int32(bss.U32(eff[:], off)), true
}

type buffStat struct {
	id    model.StatId
	label string
}

func canonicalBuffStat(id model.StatId) buffStat {
	return buffStat{id: id}
}

func displayBuffStat(id model.StatId, label string) buffStat {
	return buffStat{id: id, label: label}
}

func namedBuffStat(label string) buffStat {
	return buffStat{label: label}
}

func (s buffStat) Label() string {
	if s.label != "" {
		return s.label
	}
	if s.id != "" {
		return s.id.Label()
	}
	return ""
}

// buffModule is one module's signature: which slot holds the amount, which
// slots select the canonical stat, and how the amount is scaled/displayed.
type buffModule struct {
	stat       buffStat
	valueSlot  int     // slot index of the amount
	scale      float64 // stored amount = shown amount × scale
	unit       string  // "%" for percent stats
	negate     bool    // stored magnitude represents a reduction (shown "-")
	paramSlots []int   // slot indices forming the variant key
	variants   map[string]buffStat
}

var buffModules = map[byte]buffModule{
	// flat resource stats: (amount) in slot 0. Modules 1-6 are the client's own
	// CppEnums.BuffType_{Current,Max,Regen}{Hp,Mp}VariationAmount — the one place
	// the enum leaks into lua, confirming ModuleType is internally "BuffType".
	2: {stat: canonicalBuffStat(model.StatIdMaxHp), valueSlot: 0, scale: 1},
	3: {stat: canonicalBuffStat(model.StatIdHpRecovery), valueSlot: 0, scale: 1},
	5: {stat: canonicalBuffStat(model.StatIdMaxResource), valueSlot: 0, scale: 1},
	6: {stat: canonicalBuffStat(model.StatIdResourceRecovery), valueSlot: 0, scale: 1},
	8: {stat: canonicalBuffStat(model.StatIdMaxStamina), valueSlot: 0, scale: 1},

	// weight limit: (amount×10000) in slot 0, shown in LT ("+20LT" stored 200000)
	29: {stat: canonicalBuffStat(model.StatIdWeightLimit), valueSlot: 0, scale: 10000, unit: "LT"},

	// percent stats: (amount%) in slot 0
	9:   {stat: canonicalBuffStat(model.StatIdMoveSpeed), valueSlot: 0, scale: 10000, unit: "%"},
	10:  {stat: canonicalBuffStat(model.StatIdAttackSpeed), valueSlot: 0, scale: 10000, unit: "%"},
	11:  {stat: canonicalBuffStat(model.StatIdCastingSpeed), valueSlot: 0, scale: 10000, unit: "%"},
	30:  {stat: canonicalBuffStat(model.StatIdCritChance), valueSlot: 0, scale: 10000, unit: "%"},
	47:  {stat: canonicalBuffStat(model.StatIdHorseCaptureRate), valueSlot: 0, scale: 10000, unit: "%"},
	50:  {stat: canonicalBuffStat(model.StatIdMountExp), valueSlot: 0, scale: 10000, unit: "%"},
	51:  {stat: canonicalBuffStat(model.StatIdMountSkillExp), valueSlot: 0, scale: 10000, unit: "%"},
	56:  {stat: canonicalBuffStat(model.StatIdAmity), valueSlot: 0, scale: 10000, unit: "%"},
	57:  {stat: canonicalBuffStat(model.StatIdItemDropRate), valueSlot: 0, scale: 10000, unit: "%"},
	91:  {stat: canonicalBuffStat(model.StatIdDurabilityLossResistance), valueSlot: 0, scale: 10000, unit: "%"},
	104: {stat: canonicalBuffStat(model.StatIdKarmaRecovery), valueSlot: 0, scale: 10000, unit: "%"},
	107: {stat: canonicalBuffStat(model.StatIdGatheringDropRate), valueSlot: 0, scale: 10000, unit: "%"},
	108: {stat: canonicalBuffStat(model.StatIdKnowledgeChance), valueSlot: 0, scale: 10000, unit: "%"},
	109: {stat: canonicalBuffStat(model.StatIdHigherGradeKnowledgeChance), valueSlot: 0, scale: 10000, unit: "%"},
	112: {stat: canonicalBuffStat(model.StatIdItemDropAmount), valueSlot: 0, scale: 10000, unit: "%"},
	121: {stat: displayBuffStat(model.StatIdAutoFishingTime, "Auto-fishing Time"), valueSlot: 0, scale: 10000, unit: "%", negate: true},
	129: {stat: displayBuffStat(model.StatIdBlackSpiritRage, "Self-obtainable Black Spirit's Rage"), valueSlot: 0, scale: 100, unit: "%"},
	134: {stat: canonicalBuffStat(model.StatIdSwimmingSpeed), valueSlot: 0, scale: 10000, unit: "%"},

	// flat misc: (amount) in slot 0
	59: {stat: canonicalBuffStat(model.StatIdJumpHeight), valueSlot: 0, scale: 1},
	63: {stat: canonicalBuffStat(model.StatIdWorkerStaminaRecovery), valueSlot: 0, scale: 1},
	66: {stat: canonicalBuffStat(model.StatIdEnergyRecovery), valueSlot: 0, scale: 1},
	79: {stat: canonicalBuffStat(model.StatIdEnergyRecovery), valueSlot: 0, scale: 1},
	94: {stat: canonicalBuffStat(model.StatIdMaxEnergy), valueSlot: 0, scale: 1},

	// death-penalty resistance: (amount%) in slot 0
	90: {stat: canonicalBuffStat(model.StatIdDeathPenaltyResistance), valueSlot: 0, scale: 10000, unit: "%"},

	// combat stat families: (target, amount) — slot 0 selects
	// 0 melee/base, 1 ranged, 2 magic, 3 all
	39: {
		valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdMeleeAp), "1": canonicalBuffStat(model.StatIdRangedAp),
			"2": canonicalBuffStat(model.StatIdMagicAp), "3": displayBuffStat(model.StatIdHiddenAp, "All AP"),
		},
	},
	40: {
		valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdMeleeAccuracy), "1": canonicalBuffStat(model.StatIdRangedAccuracy),
			"2": canonicalBuffStat(model.StatIdMagicAccuracy), "3": displayBuffStat(model.StatIdAccuracy, "All Accuracy"),
		},
	},
	41: {
		valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdMeleeEvasion), "1": canonicalBuffStat(model.StatIdRangedEvasion),
			"2": canonicalBuffStat(model.StatIdMagicEvasion), "3": displayBuffStat(model.StatIdEvasion, "All Evasion"),
		},
	},
	43: {
		valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdMeleeDamageReduction), "1": canonicalBuffStat(model.StatIdRangedDamageReduction),
			"2": canonicalBuffStat(model.StatIdMagicDamageReduction), "3": displayBuffStat(model.StatIdDamageReduction, "All Damage Reduction"),
		},
	},
	46: {
		valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdHumanAp), "1": canonicalBuffStat(model.StatIdDemihumanAp),
			"2": canonicalBuffStat(model.StatIdBeastAp), "3": displayBuffStat(model.StatIdKamasylvianAp, "Extra AP Against Kamasylvian Monsters"),
			"4": canonicalBuffStat(model.StatIdEdaniaAp),
		},
	},

	// crowd-control resistances: (kind, amount%)
	49: {
		valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdKnockbackResistance), "1": canonicalBuffStat(model.StatIdKnockdownResistance),
			"2": canonicalBuffStat(model.StatIdGrappleResistance), "3": canonicalBuffStat(model.StatIdStunResistance),
			"5": canonicalBuffStat(model.StatIdStunResistance), "7": canonicalBuffStat(model.StatIdFearResistance),
			"8": canonicalBuffStat(model.StatIdAllResistance),
		},
	},
	105: {
		valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdIgnoreKnockbackResistance), "1": canonicalBuffStat(model.StatIdIgnoreKnockdownResistance),
			"2": canonicalBuffStat(model.StatIdIgnoreGrappleResistance), "3": canonicalBuffStat(model.StatIdIgnoreStunResistance),
			"8": canonicalBuffStat(model.StatIdIgnoreAllResistance),
		},
	},

	// EXP gain: (amount%, kind, lifeSkill) — kind: 0 Combat / 1 Skill / 2 life
	25: {valueSlot: 0, scale: 10000, unit: "%", paramSlots: []int{1, 2}, variants: expVariants()},

	// life-skill EXP, flat item variant: (lifeSkill, amount)
	80: {valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: lifeSkillVariants(" EXP")},

	// family fitness experience: (kind, amount, rule) — kind 0 Breath,
	// 1 Strength, 2 Health. The optional rule flag does not change the amount.
	89: {
		valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdBreathExp), "1": canonicalBuffStat(model.StatIdStrengthExp),
			"2": canonicalBuffStat(model.StatIdHealthExp),
		},
	},

	// life-skill mastery: (lifeSkill, _, amount)
	149: {valueSlot: 2, scale: 1, paramSlots: []int{0}, variants: masteryVariants()},

	// potential levels: (kind, ranks)
	67: {
		valueSlot: 1, scale: 1, paramSlots: []int{0}, variants: map[string]buffStat{
			"0": displayBuffStat(model.StatIdMovementSpeedLevel, "Movement Speed"), "1": displayBuffStat(model.StatIdAttackSpeedLevel, "Attack Speed"),
			"2": displayBuffStat(model.StatIdCastingSpeedLevel, "Casting Speed"), "3": canonicalBuffStat(model.StatIdCritLevel),
			"4": canonicalBuffStat(model.StatIdLuck), "5": canonicalBuffStat(model.StatIdFishingSpeed),
			"6": canonicalBuffStat(model.StatIdGatheringSpeed),
		},
	},

	// special-attack damage: (kind, amount%)
	93: {
		valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdSpecialAttackDamage), "1": displayBuffStat(model.StatIdBackAttackDamage, "Back Attack Extra Damage"),
			"2": displayBuffStat(model.StatIdDownAttackDamage, "Down Attack Extra Damage"), "3": displayBuffStat(model.StatIdAirAttackDamage, "Air Attack Extra Damage"),
			"4": displayBuffStat(model.StatIdCritDamage, "Critical Hit Extra Damage"), "5": displayBuffStat(model.StatIdSpeedAttackDamage, "Speed Attack Extra Damage"),
			"6": displayBuffStat(model.StatIdCounterAttackDamage, "Counter Attack Extra Damage"),
		},
	},

	// weather resistances: (kind, amount%) — kind 0 heat, 1 cold
	128: {
		valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdHeatstrokeResistance), "1": canonicalBuffStat(model.StatIdHypothermiaResistance),
		},
	},

	// underwater breathing: (ms) in slot 0, shown in seconds
	95: {stat: canonicalBuffStat(model.StatIdUnderwaterBreathing), valueSlot: 0, scale: 1000, unit: "sec"},

	// assorted parameterized percent stats: (kind, amount%)
	98:  {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{"1": canonicalBuffStat(model.StatIdMoveSpeed)}},
	106: {valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{"3": displayBuffStat(model.StatIdDamageReductionRate, "All Damage Reduction")}},
	126: {
		valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{
			"1": displayBuffStat(model.StatIdRareFishChance, "Chance to Catch Rare Fish"), "2": displayBuffStat(model.StatIdRareFishChance, "Chance to Catch Rare Fish"),
		},
	},
	181: {
		valueSlot: 1, scale: 10000, unit: "%", paramSlots: []int{0}, variants: map[string]buffStat{
			"0": canonicalBuffStat(model.StatIdBreathExp), "1": canonicalBuffStat(model.StatIdStrengthExp),
		},
	},

	// O'dyllita sun/lunar/earth food buffs: (_, amount, which)
	187: {
		valueSlot: 1, scale: 100, paramSlots: []int{2}, variants: map[string]buffStat{
			"0": namedBuffStat("Sun Buff"), "1": namedBuffStat("Lunar Buff"), "2": namedBuffStat("Earth Buff"),
		},
	},
}

// expVariants builds module 25's variant table: arg1 = 0 Combat / 1 Skill /
// 2 life-skill EXP with arg2 selecting the life skill.
func expVariants() map[string]buffStat {
	v := map[string]buffStat{
		"0,0": canonicalBuffStat(model.StatIdCombatExp),
		"1,0": canonicalBuffStat(model.StatIdSkillExp),
	}
	for _, info := range model.LifeSkillTypes.Infos() {
		if info.ExpStat != "" {
			v[fmt.Sprintf("2,%d", info.Wire())] = canonicalBuffStat(info.ExpStat)
		}
	}
	v[fmt.Sprintf("2,%d", model.LifeSkillTypeCount)] = canonicalBuffStat(model.StatIdLifeExp)
	return v
}

func lifeSkillVariants(_ string) map[string]buffStat {
	v := map[string]buffStat{}
	for _, info := range model.LifeSkillTypes.Infos() {
		if info.ExpStat != "" {
			v[fmt.Sprintf("%d", info.Wire())] = canonicalBuffStat(info.ExpStat)
		}
	}
	v[fmt.Sprintf("%d", model.LifeSkillTypeCount)] = canonicalBuffStat(model.StatIdLifeExp)
	return v
}

func masteryVariants() map[string]buffStat {
	v := map[string]buffStat{}
	for _, info := range model.LifeSkillTypes.Infos() {
		if info.MasteryStat != "" {
			v[fmt.Sprintf("%d", info.Wire())] = canonicalBuffStat(info.MasteryStat)
		}
	}
	v[fmt.Sprintf("%d", model.LifeSkillTypeCount)] = displayBuffStat(model.StatIdAllMastery, "Life Skill Mastery")
	return v
}

// customModules handles the modules whose value slot, scale, or unit is chosen
// by a parameter — so the variants can't share one fixed layout. Each returns
// (stat, unit, raw stored value, scale, ok); ok=false means "not a displayable
// effect" (e.g. a module-1 damage-over-time debuff) and the caller moves on.
var customModules = map[byte]func(eff [92]byte, cond int16) (stat buffStat, unit string, raw int32, scale float64, ok bool){
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
func onHitRecovery(eff [92]byte, cond int16, hit, critical model.StatId) (stat buffStat, unit string, raw int32, scale float64, ok bool) {
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
		return canonicalBuffStat(hit), "", v0, 1, true
	case 9:
		return canonicalBuffStat(critical), "", v0, 1, true
	}
	return
}

func resolveHpOnHit(eff [92]byte, cond int16) (stat buffStat, unit string, raw int32, scale float64, ok bool) {
	if s, u, r, sc, o := onHitRecovery(eff, cond, model.StatIdHpRecoveryOnHit, model.StatIdHpRecoveryOnCriticalHit); o {
		return s, u, r, sc, o
	}
	return onHitFixedDamage(eff, cond)
}

// onHitFixedDamage: module 1 with a negative slot-0 value is the offensive twin
// of HP recovery — it deals fixed damage on a trigger rather than healing. The
// amount is shown positive (damage dealt to the target, not a debuff on the
// wearer); ConditionType picks the trigger. cond 0/2 (DoTs, one-off drains) are
// monster/debuff buffs, not item stats, so they fall through unresolved.
func onHitFixedDamage(eff [92]byte, cond int16) (stat buffStat, unit string, raw int32, scale float64, ok bool) {
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
		return namedBuffStat("Fixed Damage on Back Attack Hits"), "", -v0, 1, true
	case 10:
		return namedBuffStat("Fixed Damage on Critical Hits"), "", -v0, 1, true
	case 4:
		return namedBuffStat("Retaliation Fixed Damage when Struck"), "", -v0, 1, true
	}
	return
}

func resolveMpOnHit(eff [92]byte, cond int16) (stat buffStat, unit string, raw int32, scale float64, ok bool) {
	return onHitRecovery(eff, cond, model.StatIdResourceRecoveryOnHit, model.StatIdResourceRecoveryOnCriticalHit)
}

// resolveManufacture: value always in slot 1; slot 0 selects the effect and its
// scale/unit — a life-skill time reduction (seconds, shown negative) or the
// processing success-rate percent.
func resolveManufacture(eff [92]byte, _ int16) (stat buffStat, unit string, raw int32, scale float64, ok bool) {
	v1, _ := effectArg(eff, 1)
	if v1 == 0 {
		return
	}
	switch v0, _ := effectArg(eff, 0); v0 {
	case 0:
		return canonicalBuffStat(model.StatIdAlchemyTime), "sec", -v1, 50000, true // reduction -> negative
	case 1:
		return canonicalBuffStat(model.StatIdCookingTime), "sec", -v1, 50000, true
	case 2:
		return canonicalBuffStat(model.StatIdProcessingSuccessRate), "%", v1, 10000, true
	}
	return
}

// resolveMonsterDR: value always in slot 1; slot 0 selects flat vs percent rate.
func resolveMonsterDR(eff [92]byte, _ int16) (stat buffStat, unit string, raw int32, scale float64, ok bool) {
	v1, _ := effectArg(eff, 1)
	if v1 == 0 {
		return
	}
	switch v0, _ := effectArg(eff, 0); v0 {
	case 0:
		return canonicalBuffStat(model.StatIdMonsterDamageReductionRate), "%", v1, 10000, true
	case 2:
		return canonicalBuffStat(model.StatIdMonsterDamageReduction), "", v1, 1, true
	}
	return
}

// resolveMonsterExtra: the value's slot IS the variant — slot 0 = vs Monsters,
// slot 1 = vs Adventurers (both flat).
func resolveMonsterExtra(eff [92]byte, _ int16) (stat buffStat, unit string, raw int32, scale float64, ok bool) {
	if v, _ := effectArg(eff, 0); v != 0 {
		return canonicalBuffStat(model.StatIdMonsterAp), "", v, 1, true
	}
	if v, _ := effectArg(eff, 1); v != 0 {
		return canonicalBuffStat(model.StatIdAdventurerAp), "", v, 1, true
	}
	return
}

// ResolvedBuffStat is one effect decoded from a buff module's typed arguments.
type ResolvedBuffStat struct {
	ID    model.StatId
	Label string
	Op    string
	Value float64
	Unit  string
}

// ResolveBuffStat decodes a buff's effect from its module arguments. The
// canonical ID is the source of truth; Label is derived from it when available.
func ResolveBuffStat(b Buff) (ResolvedBuffStat, bool) {
	if fn, cok := customModules[b.Module]; cok {
		st, unit, raw, scale, rok := fn(b.EffectData, b.Condition)
		if !rok || raw == 0 {
			return ResolvedBuffStat{}, false
		}
		v := float64(raw) / scale
		o := "+"
		if v < 0 {
			o, v = "-", -v
		}
		return ResolvedBuffStat{ID: st.id, Label: st.Label(), Op: o, Value: v, Unit: unit}, true
	}
	m, has := buffModules[b.Module]
	if !has {
		return ResolvedBuffStat{}, false
	}
	raw, valid := effectArg(b.EffectData, m.valueSlot)
	if !valid || raw == 0 {
		return ResolvedBuffStat{}, false
	}
	stat := m.stat
	if len(m.paramSlots) > 0 {
		keys := make([]string, len(m.paramSlots))
		for i, slot := range m.paramSlots {
			p, pok := effectArg(b.EffectData, slot)
			if !pok {
				return ResolvedBuffStat{}, false
			}
			keys[i] = fmt.Sprintf("%d", uint32(p))
		}
		stat = m.variants[strings.Join(keys, ",")]
		if stat.id == "" && stat.label == "" {
			return ResolvedBuffStat{}, false
		}
	}
	val := float64(raw) / m.scale
	op := "+"
	if m.negate {
		op = "-"
	}
	if val < 0 {
		op, val = "-", -val
	}
	return ResolvedBuffStat{ID: stat.id, Label: stat.Label(), Op: op, Value: val, Unit: m.unit}, true
}
