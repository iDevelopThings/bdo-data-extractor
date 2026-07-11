package tables

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// maxLevel guards against the formula/set key-space (real enchant levels run 0..~25).
const maxLevel = 30

// clampStat zeroes the 0x02000000 "no value" sentinel, NaN/Inf, and absurd
// magnitudes (misaligned reads / DSL text read as float).
func clampStat(v float64) float64 {
	if math.Abs(v) < 1e-6 || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e7 {
		return 0
	}
	return v
}

// triMax reads a tri-dice stat — melee/ranged/magic slots side by side — and returns
// the largest. The slots are exclusive per weapon type (a sword fills melee, a staff
// magic; hybrids fill two equally), so the max is the gear's real value.
func triMax(c *bss.Cursor) float64 {
	a, b, d := clampStat(c.F32()), clampStat(c.F32()), clampStat(c.F32())
	if b > a {
		a = b
	}
	if d > a {
		a = d
	}
	return a
}

// Func names are usually SCREAMING_CASE but some are mixed-case (e.g.
// Donkey_Harness_SET_EFFECT_1_2), so allow lowercase — safe because we now parse the
// clean length-prefixed DSL string, not junk decoded from the whole record.
var formulaRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_]*)\(([^)]*)\)`)

// dslSeparators are everything that legitimately sits between formulas in the DSL
// (formula terminators + whitespace); after removing every matched formula, only
// these may remain.
var dslSeparators = strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "", ";", "", ":", "").Replace

// bakedEffectValues supplies the magnitude for the few argless "named effect" DSL
// funcs whose value is fixed in the client rather than carried in the data. Unlike
// the generic ALL_REG_ADD(n) (value in the arg, e.g. Labreska's Helmet = +5%), these
// family-specific variants ship no arg, so we fill in the confirmed constant — the
// output keeps the same {func, args} shape, the arg just comes from here. Only add a
// func once a tooltip confirms its value; leave the rest arg-less.
var bakedEffectValues = map[string][]float64{
	"NU_ALL_REG_ADD": {10}, // Nouver — "All Resistance +10%"
	"KU_ALL_REG_ADD": {10}, // Kutum  — "All Resistance +10%"
}

// parseFormulas pulls every NAME(args) formula (item + set effects) out of the
// record's effect DSL string. It validated across all 30,813 DSL records:
//   - func names can be mixed-case (Donkey_Harness_SET_EFFECT_1_2) → formulaRe allows lowercase
//   - args can be fractional (ALCHEMY_REDUCE_TIME_DOWN(0.7)) → Effect.Args is []float64
//   - args can be roman numerals (MERMAID_HOPE_ADD(IV), the tier) → converted to their int value
//
// A completeness guard removes every matched formula from the DSL and fatals if
// anything but separators is left (a formula the regex missed) or if an arg parses
// as neither number nor roman numeral — so a silent drop can't slip in on a patch.
func parseFormulas(dsl string) []model.EffectGroup {
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
						log.Fatalf("parseFormulas: un-parseable arg %q in %q", a, m[0])
					}
					v = float64(r)
				}
				e.Args = append(e.Args, v)
			}
		} else if v, ok := bakedEffectValues[m[1]]; ok {
			e.Args = v // argless in the data, but a known client-side constant
		}
		dsls = append(dsls, e)
	}
	if leftover := dslSeparators(remainder); leftover != "" {
		log.Fatalf("parseFormulas: unmatched DSL content %q (leftover %q)", dsl, leftover)
	}

	return model.FormatEffectFunctions(
		dsls,
		true,
		"",
	)
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

// DecodeEnchantCurves decodes enchantstaticstatus into per-baseId curves.
// Record key = (level<<24) | baseId.
func DecodeEnchantCurves(offsetRaw, dataRaw []byte) (map[uint32]*model.Enhancement, error) {
	idx, err := bss.ParseOffsetIndex(offsetRaw, len(dataRaw))
	if err != nil {
		return nil, err
	}
	out := map[uint32]*model.Enhancement{}
	for _, e := range idx {
		level := int(e.Key >> 24)
		base := e.Key & 0xFFFFFF
		if level > maxLevel {
			continue
		}
		rec, ok := e.Slice(dataRaw)
		if !ok || len(rec) < 263 { // need the fixed header through the DSL length prefix
			fmt.Printf("DecodeEnchantCurves: skipping baseId %d level %d, record too short (%d bytes)\n", base, level, len(rec))
			continue
		}
		lv := decodeEnchantRow(rec, level)
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
//	@0   u32 baseId (== key)   @4  u32   @8 u32   @12 u32 ×3   @24 u8 (shift)
//	@25  u32 ×4   @41 u32   @45 u32   @49 u32
//	@53  u16 durability   @55 u16   @57 u16   @59 u8 (shift)   @60 u16
//	@62  f32 maxHP   @66 f32 ×25 (per-species AP, ~0)
//	@166 u8   @167 tri-dice ×7: [_, _, apMin, apMax, apDisplay, DR, Evasion]
//	@251 u32   @255 i64   @263 [i64 len][UTF-16 effect DSL]
//	then: u8·u8·u32·u32·u8·u8·u8 · accuracy 3×[i64 dice-len][dice][f32] ·
//	      defense 3×[f32 evasion][f32 addedEvasion][f32 DR][f32 addedDR] · footer
//
// AP/DR/Evasion/maxHP land here identically to the old fixed offsets; accuracy and
// the added-defense values live ONLY in the post-DSL tail (missed entirely before).
// Unidentified scalars are captured as EnchantUnknowns (nil when zero). If the
// cursor overruns mid-tail (a record with a different layout) the tail stats stay
// zero rather than emitting garbage.
func decodeEnchantRow(rec []byte, level int) model.EnchantLevel {
	c := bss.NewCursor(rec, 0, len(rec))

	// Header, read largest-type-first: everything is u32 except the u16 enhancement
	// block (@53-60, where a u32 would straddle two fields) and the two lone shift
	// bytes (@24, @59) that a u32 read would overrun. Widths were derived by reading
	// wide and shrinking only where the next field started landing on garbage.
	c.U32()                     // @0  baseId (== key)
	u4 := int(c.U32())          // @4
	u8 := int(c.U32())          // @8
	c.U32()                     // @12
	c.U32()                     // @16
	c.U32()                     // @20
	c.U8()                      // @24 shift byte
	u25 := int(c.U32())         // @25
	c.U32()                     // @29
	c.U32()                     // @33
	c.U32()                     // @37
	chanceRaw := int(c.U32())   // @41 enhance success chance ×1e6
	u45 := int(c.U32())         // @45
	c.U32()                     // @49
	dura := int(c.U16())        // @53 durability (base 100, rises to PEN 200)
	u55 := int(c.U16())         // @55 (rises with enhancement)
	u57 := int(c.U16())         // @57
	c.U8()                      // @59 shift byte
	u60 := int(c.U16())         // @60 (rises with enhancement)
	maxHP := clampStat(c.F32()) // @62
	var slot70 float64          // @66..165 per-species-AP band; only slot 1 (@70) is ever non-zero
	for i := 0; i < 25; i++ {
		if v := clampStat(c.F32()); i == 1 {
			slot70 = v
		}
	}
	c.U8()                              // @166 flag
	u167 := rnd(triMax(c))              // @167 (tracks minAP-1)
	triMax(c)                           // @179 empty block
	apMin := rnd(triMax(c))             // @191
	apMax := rnd(triMax(c))             // @203
	apDisp := rnd(triMax(c))            // @215
	dr := rnd(triMax(c))                // @227
	eva := rnd(triMax(c))               // @239
	c.U32()                             // @251 (structural marker)
	c.I64()                             // @255
	effects := parseFormulas(c.UTF16()) // @263 length-prefixed effect DSL

	// post-DSL display-stat tail
	c.U8()
	c.U8()                // 01, 03
	rate1 := int(c.U32()) // ~1,000,000
	rate2 := int(c.U32()) // ~700,000
	c.U8()
	c.U8()
	c.U8()
	var acc float64
	for i := 0; i < 3; i++ { // accuracy: 3× [dice string][value]
		c.UTF16()
		if v := clampStat(c.F32()); v > acc {
			acc = v
		}
	}
	var addEva, addDR float64
	for i := 0; i < 3; i++ { // defense: 3× [evasion][addedEvasion][DR][addedDR]
		c.F32()
		if v := clampStat(c.F32()); v > addEva {
			addEva = v
		}
		c.F32()
		if v := clampStat(c.F32()); v > addDR {
			addDR = v
		}
	}
	tailOK := c.OK()

	lv := model.EnchantLevel{
		Level:           level,
		ApMin:           apMin,
		ApMax:           apMax,
		Evasion:         eva,
		DamageReduction: dr,
		MaxHP:           rnd(maxHP),
		Durability:      dura,
		Effects:         effects,
	}
	if apMin != 0 || apMax != 0 {
		if apDisp != 0 { // prefer the game's own display AP
			lv.Ap = apDisp
		} else {
			lv.Ap = rnd((float64(apMin) + float64(apMax)) / 2)
		}
	}
	if eva != 0 || dr != 0 {
		lv.Dp = eva + dr
	}
	if chanceRaw > 0 && chanceRaw <= 1_000_000 { // base enhance success chance (@41 ÷ 1e6)
		lv.EnhanceChance = float64(chanceRaw) / 1_000_000
	}
	if tailOK {
		lv.Accuracy = rnd(acc)
		lv.AddedEvasion = rnd(addEva)
		lv.AddedDamageReduction = rnd(addDR)
		lv.UnknownRate1 = dev(rate1, 0)
		lv.UnknownRate2 = dev(rate2, 0)
	}
	lv.Unknown4 = dev(u4, 0)
	lv.Unknown8 = dev(u8, 0)
	lv.Unknown25 = dev(u25, 0)
	lv.Unknown45 = dev(u45, 0)
	lv.Unknown55 = dev(u55, 0)
	lv.Unknown57 = dev(u57, 0)
	lv.Unknown60 = dev(u60, 0)
	lv.Unknown70 = dev(rnd(slot70), 0)
	lv.Unknown167 = dev(u167, 0)
	return lv
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
