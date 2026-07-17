package tables

import (
	"fmt"
	"math"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// cronenchant.bss — the Caphras Enhancement chart ("CronEnchant" is the game's
// internal name for the Caphras system; the UI Lua reads it via
// ToClient_GetCronEnchantWrapper(cronKey, enchantLevel, gradeIndex)).
//
// PABR, 10 fixed-size rows (one per equipment category / cronKey 1..10), no
// string table. Row layout:
//
//	u32 groupCount (3)                     — enhancement levels 18/19/20
//	3 × [u32 stepCount (20)][20 × entry]   — the 20 Caphras levels per group
//
//	entry (39 bytes):
//	  u8  cronKey        == the row's category key (1..10)
//	  u8  0
//	  u8  enchantLevel   18 (TRI) / 19 (TET) / 20 (PEN)
//	  u32 totalStones    cumulative Caphras Stones to REACH this level
//	  7 × f32 + u32      added stats at this level, in the wrapper's getter
//	                     order: DD (AP), HIT (accuracy), DV (evasion),
//	                     HDV (hidden evasion), PV (damage reduction),
//	                     HPV (hidden DR), MaxHP, MaxMP
//
// Validated against live-game/questlog ground truth: category 7/8 PEN =
// 153/383/690/… stones with L1 = evasion+1, hidden evasion+1, HP+20 (the PEN
// boss-armor values), L20 total = 21744; category 10 PEN = 28/71/128/….
//
// The item→cronKey mapping is NOT stored client-side (computed in the exe);
// the build derives it from slot kind × buy-price tier — see FORMATS §16.

// caphrasEffectFuncs maps the entry's eight stat columns (the CronEnchant
// wrapper's getter order) onto enhancement-DSL function names, so the steps
// emit model.Effect entries in the same shape as EnchantLevel.Effects.
var caphrasEffectFuncs = [8]string{
	"ALL_AP_INCRE",            // getAddedDD
	"ALL_HIT_INCRE",           // getAddedHIT
	"ALL_EVA_INCRE",           // getAddedDV
	"HIDDEN_EVA_INCRE",        // getAddedHDV (extension name; no DSL equivalent)
	"ALL_DAM_REDUCE_INCRE",    // getAddedPV
	"HIDDEN_DAM_REDUCE_INCRE", // getAddedHPV (extension name)
	"HP_UP",                   // getAddedMaxHP
	"MP_WP_SP_UP",             // getAddedMaxMP
}

func DecodeCaphras(data []byte) ([]model.CaphrasCategory, error) {
	h, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("cronenchant: %w", err)
	}
	rowSize, ok := h.RecordSize()
	if !ok {
		return nil, fmt.Errorf("cronenchant: records don't tile evenly (rows=%d)", h.Rows)
	}
	out := make([]model.CaphrasCategory, 0, h.Rows)
	for rowIndex := 0; rowIndex < h.Rows; rowIndex++ {
		start := h.RecordsStart + rowIndex*rowSize
		cat, err := decodeCaphrasRow(data[start : start+rowSize])
		if err != nil {
			return nil, fmt.Errorf("cronenchant: row %d: %w", rowIndex, err)
		}
		out = append(out, cat)
	}
	return out, nil
}

func decodeCaphrasRow(record []byte) (model.CaphrasCategory, error) {
	c := bss.NewCursor(record, 0, len(record))
	groupCount := int(c.U32())
	if groupCount <= 0 || groupCount > 16 {
		return model.CaphrasCategory{}, fmt.Errorf("invalid group count %d", groupCount)
	}

	categoryKey := 0
	seenLevels := make(map[int]bool, groupCount)
	levels := make([]model.CaphrasEnhancement, 0, groupCount)
	for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
		stepCount := int(c.U32())
		if stepCount <= 0 || stepCount > 100 {
			return model.CaphrasCategory{}, fmt.Errorf("group %d has invalid step count %d", groupIndex, stepCount)
		}

		level := 0
		previousTotal := 0
		steps := make([]model.CaphrasLevel, 0, stepCount)
		for stepIndex := 0; stepIndex < stepCount; stepIndex++ {
			key := c.U8()
			reserved := c.U8()
			enchantLevel := c.U8()
			total := int(c.U32())
			stats := model.CaphrasStats{
				AP:                    c.F32(),
				Accuracy:              c.F32(),
				Evasion:               c.F32(),
				HiddenEvasion:         c.F32(),
				DamageReduction:       c.F32(),
				HiddenDamageReduction: c.F32(),
				MaxHP:                 c.F32(),
				MaxMP:                 int(c.U32()),
			}
			if !c.OK() {
				return model.CaphrasCategory{}, fmt.Errorf("group %d step %d is truncated", groupIndex, stepIndex)
			}
			if reserved != 0 {
				return model.CaphrasCategory{}, fmt.Errorf("group %d step %d reserved byte is %d", groupIndex, stepIndex, reserved)
			}
			if categoryKey == 0 {
				categoryKey = key
			}
			if key == 0 || key != categoryKey {
				return model.CaphrasCategory{}, fmt.Errorf("group %d step %d category key %d, want %d", groupIndex, stepIndex, key, categoryKey)
			}
			if level == 0 {
				level = enchantLevel
			}
			if enchantLevel != level {
				return model.CaphrasCategory{}, fmt.Errorf("group %d step %d enhancement level %d, want %d", groupIndex, stepIndex, enchantLevel, level)
			}
			if total < previousTotal {
				return model.CaphrasCategory{}, fmt.Errorf("group %d step %d has non-monotonic total %d", groupIndex, stepIndex, total)
			}
			if err := validateCaphrasStats(stats); err != nil {
				return model.CaphrasCategory{}, fmt.Errorf("group %d step %d: %w", groupIndex, stepIndex, err)
			}

			effects := caphrasEffects(stats)
			steps = append(steps, model.CaphrasLevel{
				Level:       stepIndex + 1,
				Stones:      total - previousTotal,
				TotalStones: total,
				Stats:       stats,
				Effects:     model.FormatEffectFunctions(effects, false, "Caphras"),
			})
			previousTotal = total
		}
		if seenLevels[level] {
			return model.CaphrasCategory{}, fmt.Errorf("duplicate enhancement level %d", level)
		}
		seenLevels[level] = true
		levels = append(levels, model.CaphrasEnhancement{EnchantLevel: level, Steps: steps})
	}
	if c.Remaining() != 0 {
		return model.CaphrasCategory{}, fmt.Errorf("%d trailing bytes at byte %d", c.Remaining(), c.Pos())
	}
	if categoryKey == 0 {
		return model.CaphrasCategory{}, fmt.Errorf("missing category key")
	}

	return model.CaphrasCategory{
		BaseFor: models.NewBaseFor[model.CaphrasCategory](uint32(categoryKey)),
		Key:     categoryKey,
		Levels:  levels,
	}, nil
}

func validateCaphrasStats(stats model.CaphrasStats) error {
	values := [...]float64{
		stats.AP, stats.Accuracy, stats.Evasion, stats.HiddenEvasion,
		stats.DamageReduction, stats.HiddenDamageReduction, stats.MaxHP,
	}
	for index, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("invalid stat %d value %v", index, value)
		}
	}
	return nil
}

func caphrasEffects(stats model.CaphrasStats) []model.EffectDsl {
	values := [...]float64{
		stats.AP, stats.Accuracy, stats.Evasion, stats.HiddenEvasion,
		stats.DamageReduction, stats.HiddenDamageReduction, stats.MaxHP, float64(stats.MaxMP),
	}
	effects := make([]model.EffectDsl, 0, len(values))
	for index, value := range values {
		if value == 0 {
			continue
		}
		effects = append(effects, model.EffectDsl{
			Func: caphrasEffectFuncs[index],
			Args: []float64{value},
		})
	}
	return effects
}
