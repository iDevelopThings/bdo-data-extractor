package tables

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// clampStat zeroes the 0x02000000 "no value" sentinel, NaN/Inf, and absurd
// magnitudes (misaligned reads / DSL text read as float).
func clampStat(v float64) float64 {
	if math.Abs(v) < 1e-6 || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e7 {
		return 0
	}
	return v
}

type enchantTri [3]float64

func readEnchantTri(c *bss.Cursor) enchantTri {
	return enchantTri{clampStat(c.F32()), clampStat(c.F32()), clampStat(c.F32())}
}

func (v enchantTri) max() float64 {
	return max(v[0], v[1], v[2])
}

// Func names are usually SCREAMING_CASE but some are mixed-case, such as
// Donkey_Harness_SET_EFFECT_1_2.
var formulaRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_]*)\(([^)]*)\)`)

// dslSeparators are everything that legitimately sits between formulas in the DSL
// (formula terminators + whitespace); after removing every matched formula, only
// these may remain.
var dslSeparators = strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "", ";", "", ":", "").Replace

// parseFormulas pulls every NAME(args) formula (item + set effects) out of the
// record's effect DSL string. The table contains:
//   - func names can be mixed-case (Donkey_Harness_SET_EFFECT_1_2) → formulaRe allows lowercase
//   - args can be fractional (ALCHEMY_REDUCE_TIME_DOWN(0.7)) → Effect.Args is []float64
//   - args can be roman numerals (MERMAID_HOPE_ADD(IV), the tier) → converted to their int value
//
// A completeness guard removes every matched formula from the DSL and returns an
// error if anything but separators remains or an argument is not understood.
func parseFormulas(dsl string) ([]model.EffectGroup, error) {
	var dsls []model.EffectDsl

	remainder := dsl
	for _, m := range formulaRe.FindAllStringSubmatch(dsl, -1) {
		remainder = strings.Replace(remainder, m[0], "", 1)
		e := model.EffectDsl{Func: m[1]}
		if arg := strings.TrimSpace(m[2]); arg != "" {
			for _, a := range strings.Split(arg, ",") {
				a = strings.TrimSpace(a)
				v, err := strconv.ParseFloat(a, 64)
				if err != nil {
					r, ok := romanToInt(a)
					if !ok {
						return nil, fmt.Errorf("effect DSL: unparseable argument %q in %q", a, m[0])
					}
					v = float64(r)
				}
				e.Args = append(e.Args, v)
			}
		}
		dsls = append(dsls, e)
	}
	if leftover := dslSeparators(remainder); leftover != "" {
		return nil, fmt.Errorf("effect DSL: unmatched content %q in %q", leftover, dsl)
	}

	return model.FormatEffectFunctions(
		dsls,
		true,
		"",
	), nil
}

// romanToInt converts a roman numeral (I, IV, …) to its value; ok is false for any
// non-roman string.
func romanToInt(s string) (val int, ok bool) {
	numeral := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	prev := 0
	for i := len(s) - 1; i >= 0; i-- {
		v, has := numeral[s[i]]
		if !has {
			return 0, false
		}
		if v < prev {
			val -= v
		} else {
			val += v
			prev = v
		}
	}
	return val, val > 0
}

// DecodeEnchantCurves decodes enchantstaticstatus into per-baseID curves.
// Record key = (level<<24) | baseID; bits 16..23 are reserved and zero.
func DecodeEnchantCurves(offsetRaw, dataRaw []byte) (map[uint32]*model.Enhancement, error) {
	idx, err := bss.ParseOffsetIndex(offsetRaw, len(dataRaw))
	if err != nil {
		return nil, err
	}
	out := map[uint32]*model.Enhancement{}
	for _, e := range idx {
		level := int(e.Key >> 24)
		base := e.Key & 0xFFFF
		if e.Key&0x00FF0000 != 0 {
			return nil, fmt.Errorf("enchantstaticstatus: key %08x has nonzero reserved byte", e.Key)
		}
		rec, ok := e.Slice(dataRaw)
		if !ok {
			return nil, fmt.Errorf("enchantstaticstatus: baseID %d level %d is out of bounds", base, level)
		}
		lv, err := decodeEnchantRow(rec, e.Key)
		if err != nil {
			return nil, fmt.Errorf("enchantstaticstatus: baseID %d level %d: %w", base, level, err)
		}
		c := out[base]
		if c == nil {
			c = &model.Enhancement{
				BaseID: base,
			}
			out[base] = c
		}
		c.Levels = append(c.Levels, lv)
	}
	for _, c := range out {
		sort.Slice(c.Levels, func(i, j int) bool { return c.Levels[i].Level < c.Levels[j].Level })

		c.MinLevel, c.MaxLevel = math.MaxInt, math.MinInt
		c.MinLevelIdx, c.MaxLevelIdx = -1, -1
		for i, level := range c.Levels {
			if level.Level < c.MinLevel {
				c.MinLevel = level.Level
				c.MinLevelIdx = i
			}
			if level.Level > c.MaxLevel {
				c.MaxLevel = level.Level
				c.MaxLevelIdx = i
			}
		}

	}
	return out, nil
}

// decodeEnchantRow reads ONE enchant-curve record as a single front-to-back
// sequential field stream — no fixed offsets, no seeking, so every field is
// consumed in order and nothing is skipped or mis-anchored. Layout:
//
//	@0   u32 baseID (== key low16)   @4  u32   @8 u32   @12 u32 ×3   @24 u8
//	@25  u32 ×4   @41 u32   @45 u32   @49 u32
//	@53  u16 durability   @55 u16   @57 u16   @59 u8   @60 u16
//	@62  f32 maxHP   @66 f32 ×25 (per-species AP, ~0)
//	@166 u8   @167 tri-dice ×7: [_, _, apMin, apMax, apDisplay, DR, Evasion]
//	@251 u32   @255 [i64 len][UTF-16 source description]   then effect DSL string
//	then: u8·u8·u32·u32·u8·u8·u8 · accuracy 3×[i64 dice-len][dice][f32] ·
//	      defense 3×[f32 evasion][f32 addedEvasion][f32 DR][f32 addedDR] ·
//	      3×i32(-1) · 65-byte unknown block · [u32 count][count×u32 itemID] · 6 bytes
func decodeEnchantRow(rec []byte, key uint32) (model.EnchantLevel, error) {
	c := bss.NewCursor(rec, 0, len(rec))
	level := int(key >> 24)

	baseID := c.U32()           // @0
	u4 := int(c.U32())          // @4
	u8 := int(c.U32())          // @8
	u12 := int(c.U32())         // @12
	u16 := int(c.U32())         // @16
	u20 := int(c.U32())         // @20
	u24 := c.U8()               // @24
	u25 := int(c.U32())         // @25
	u29 := int(c.U32())         // @29
	u33 := int(c.U32())         // @33
	u37 := int(c.U32())         // @37
	chanceRaw := int(c.U32())   // @41 enhance success chance ×1e6
	u45 := int(c.U32())         // @45
	u49 := int(c.U32())         // @49
	dura := int(c.U16())        // @53 durability (base 100, rises to PEN 200)
	u55 := int(c.U16())         // @55 (rises with enhancement)
	u57 := int(c.U16())         // @57
	u59 := c.U8()               // @59
	u60 := int(c.U16())         // @60 (rises with enhancement)
	maxHP := clampStat(c.F32()) // @62
	speciesAP := make([]model.EnchantIndexedStat, 0)
	for i := 0; i < 25; i++ {
		if value := rnd(clampStat(c.F32())); value != 0 {
			speciesAP = append(speciesAP, model.EnchantIndexedStat{Index: i, Value: value})
		}
	}
	u166 := c.U8()                        // @166
	unknownAttack167 := readEnchantTri(c) // @167
	unknownAttack179 := readEnchantTri(c) // @179
	apMinTri := readEnchantTri(c)         // @191
	apMaxTri := readEnchantTri(c)         // @203
	apDisplayTri := readEnchantTri(c)     // @215
	drTri := readEnchantTri(c)            // @227
	evasionTri := readEnchantTri(c)       // @239
	u251 := int(c.U32())                  // @251
	sourceDescription := c.UTF16()        // @255 when empty; variable when populated
	effectDSL := c.UTF16()
	if !c.OK() {
		return model.EnchantLevel{}, fmt.Errorf("truncated descriptions at byte %d", c.Pos())
	}
	effects, err := parseFormulas(effectDSL)
	if err != nil {
		return model.EnchantLevel{}, err
	}

	// post-DSL display-stat tail
	display0 := c.U8()
	display1 := c.U8()
	rate1 := int(c.U32()) // ~1,000,000
	rate2 := int(c.U32()) // ~700,000
	display10 := c.U8()
	display11 := c.U8()
	display12 := c.U8()
	var accuracy enchantTri
	var accuracyDice [3]string
	for i := 0; i < 3; i++ { // accuracy: 3× [dice string][value]
		accuracyDice[i] = c.UTF16()
		accuracy[i] = clampStat(c.F32())
	}
	var defense [3][4]float64
	for i := 0; i < 3; i++ { // defense: 3× [evasion][addedEvasion][DR][addedDR]
		for field := 0; field < 4; field++ {
			defense[i][field] = clampStat(c.F32())
		}
	}
	if !c.OK() {
		return model.EnchantLevel{}, fmt.Errorf("truncated display-stat tail at byte %d", c.Pos())
	}
	if !c.Repeated(12, 0xFF) {
		return model.EnchantLevel{}, fmt.Errorf("invalid three-sentinel footer at byte %d", c.Pos()-12)
	}
	unknownTail := append([]byte(nil), c.Bytes(65)...)
	aidCount := int(c.U32())
	if aidCount < 0 || aidCount > 16 {
		return model.EnchantLevel{}, fmt.Errorf("invalid enhancement-aid count %d", aidCount)
	}
	aidIDs := c.U32N(aidCount)
	unknownFooter := append([]byte(nil), c.Bytes(6)...)
	if !c.OK() {
		return model.EnchantLevel{}, fmt.Errorf("truncated footer at byte %d", c.Pos())
	}
	if c.Remaining() != 0 {
		return model.EnchantLevel{}, fmt.Errorf("%d trailing bytes at byte %d", c.Remaining(), c.Pos())
	}
	if baseID != key&0xFFFF {
		return model.EnchantLevel{}, fmt.Errorf("record baseID %d does not match key %08x", baseID, key)
	}

	combatStats := buildEnchantCombatStats(
		unknownAttack167,
		unknownAttack179,
		apMinTri,
		apMaxTri,
		apDisplayTri,
		accuracy,
		accuracyDice,
		defense,
	)
	apMin := rnd(apMinTri.max())
	apMax := rnd(apMaxTri.max())
	apDisplay := rnd(apDisplayTri.max())
	dr := rnd(drTri.max())
	evasion := rnd(evasionTri.max())
	addedEvasion := max(rnd(defense[0][1]), rnd(defense[1][1]), rnd(defense[2][1]))
	addedDR := max(rnd(defense[0][3]), rnd(defense[1][3]), rnd(defense[2][3]))

	lv := model.EnchantLevel{
		Level:                level,
		ApMin:                apMin,
		ApMax:                apMax,
		Accuracy:             rnd(accuracy.max()),
		Evasion:              evasion,
		DamageReduction:      dr,
		AddedEvasion:         addedEvasion,
		AddedDamageReduction: addedDR,
		MaxHP:                rnd(maxHP),
		Durability:           dura,
		Effects:              effects,
		SourceDescription:    sourceDescription,
		CombatStats:          combatStats,
		SpeciesAP:            speciesAP,
		EnhancementAids:      model.ItemRefList(aidIDs...),
	}
	if apMin != 0 || apMax != 0 {
		if apDisplay != 0 { // prefer the game's own display AP
			lv.Ap = apDisplay
		} else {
			lv.Ap = rnd((float64(apMin) + float64(apMax)) / 2)
		}
	}
	if evasion != 0 || dr != 0 {
		lv.Dp = evasion + dr
	}
	if chanceRaw > 0 && chanceRaw <= 1_000_000 { // base enhance success chance (@41 ÷ 1e6)
		lv.EnhanceChance = float64(chanceRaw) / 1_000_000
	}
	lv.Unknown4 = dev(u4, 0)
	lv.Unknown8 = dev(u8, 0)
	lv.Unknown12 = dev(u12, 0)
	lv.Unknown16 = dev(u16, 0)
	lv.Unknown20 = dev(u20, 0)
	lv.Unknown24 = dev(u24, 0)
	lv.Unknown25 = dev(u25, 0)
	lv.Unknown29 = dev(u29, 0)
	lv.Unknown33 = dev(u33, 0)
	lv.Unknown37 = dev(u37, 0)
	lv.Unknown45 = dev(u45, 0)
	lv.Unknown49 = dev(u49, 0)
	lv.Unknown55 = dev(u55, 0)
	lv.Unknown57 = dev(u57, 0)
	lv.Unknown59 = dev(u59, 0)
	lv.Unknown60 = dev(u60, 0)
	lv.Unknown166 = dev(u166, 0)
	lv.Unknown251 = dev(u251, 0)
	lv.UnknownDisplay0 = dev(display0, 0)
	lv.UnknownDisplay1 = dev(display1, 0)
	lv.UnknownRate1 = dev(rate1, 1_000_000)
	lv.UnknownRate2 = dev(rate2, 700_000)
	lv.UnknownDisplay10 = dev(display10, 0)
	lv.UnknownDisplay11 = dev(display11, 0)
	lv.UnknownDisplay12 = dev(display12, 0)
	lv.UnknownTail12 = unknownTail
	if !bss.AllZero(unknownFooter) {
		lv.UnknownFooter = unknownFooter
	}
	return lv, nil
}

func buildEnchantCombatStats(
	unknownAttack167,
	unknownAttack179,
	apMin,
	apMax,
	apDisplay,
	accuracy enchantTri,
	accuracyDice [3]string,
	defense [3][4]float64,
) *model.EnchantCombatStats {
	lanes := [3]*model.EnchantCombatStat{}
	for i := range lanes {
		lane := &model.EnchantCombatStat{
			APMin:                rnd(apMin[i]),
			APMax:                rnd(apMax[i]),
			AP:                   rnd(apDisplay[i]),
			Accuracy:             rnd(accuracy[i]),
			AccuracyDice:         accuracyDice[i],
			Evasion:              rnd(defense[i][0]),
			AddedEvasion:         rnd(defense[i][1]),
			DamageReduction:      rnd(defense[i][2]),
			AddedDamageReduction: rnd(defense[i][3]),
			UnknownAttack167:     rnd(unknownAttack167[i]),
			UnknownAttack179:     rnd(unknownAttack179[i]),
		}
		if lane.AccuracyDice == "1D1-1" {
			lane.AccuracyDice = ""
		}
		if *lane != (model.EnchantCombatStat{}) {
			lanes[i] = lane
		}
	}
	if lanes == [3]*model.EnchantCombatStat{} {
		return nil
	}
	return &model.EnchantCombatStats{
		Melee:  lanes[0],
		Ranged: lanes[1],
		Magic:  lanes[2],
	}
}

// EnchantLinks resolves item id -> baseId (the item's EnchantKey), read as the
// u32 immediately before the row's Name string (see enchantKeyOf). The curve is
// attached only when its level count matches the item's max enhance level.
func EnchantLinks(ienOff, ienData []byte, curves map[uint32]*model.Enhancement, maxlevel map[uint32]int) (map[uint32]uint32, error) {
	idx, err := bss.ParseOffsetIndex(ienOff, len(ienData))
	if err != nil {
		return nil, err
	}
	links := map[uint32]uint32{}
	for _, e := range idx {
		iid := e.Key
		if iid >= maxItemID {
			continue
		}
		ml, hasML := maxlevel[iid]
		rec, ok := e.Slice(ienData)
		if !ok || len(rec) < 12 {
			continue
		}
		if bss.U32(rec, 0) != iid {
			continue
		}
		ek, ok := enchantKeyOf(rec)
		if !ok || ek == 0 {
			// the EnchantKey slot exists on EVERY row (non-Equip records share
			// the same [key][name][icon] tail) but is 0 for all 39,270
			// non-Equip items — and no baseId-0 curve exists, so 0 never links
			continue
		}
		c := curves[ek]
		if c == nil {
			continue
		}
		if hasML && ml >= 1 {
			if len(c.Levels) == ml+1 {
				links[iid] = ek
			}
			continue
		}
		// non-enhanceable equipment (artifacts, lightstones, …): a single
		// base-level curve carries the item's base stats/effects as DSL
		// (e.g. Marsh's Artifact L0 = SHORT_AP_UP(4)). The Equip check
		// (ItemType @4 == 1) is future-proofing only — non-Equip rows all
		// carry key 0 and are already skipped above.
		if rec[4] == 1 && len(c.Levels) == 1 {
			links[iid] = ek
		}
	}
	return links, nil
}
