package tables

import (
	"fmt"

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
//	  8 × f32            added stats at this level, in the wrapper's getter
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
	rows := h.Rows

	out := make([]model.CaphrasCategory, 0, rows)
	for r := 0; r < rows; r++ {
		o := 8 + r*rowSize
		end := o + rowSize
		groups := int(bss.U32(data, o))
		o += 4
		cat := model.CaphrasCategory{BaseFor: models.NewBaseFor[model.CaphrasCategory](uint32(r + 1)), Key: r + 1}
		for g := 0; g < groups; g++ {
			if o+4 > end {
				return nil, fmt.Errorf("cronenchant: row %d group %d truncated", r, g)
			}
			steps := int(bss.U32(data, o))
			o += 4
			if steps <= 0 || steps > 100 || o+steps*39 > end {
				return nil, fmt.Errorf("cronenchant: row %d group %d bad step count %d", r, g, steps)
			}
			enh := model.CaphrasEnhancement{EnchantLevel: int(data[o+2])}
			prevTotal := 0
			for e := 0; e < steps; e++ {
				key, pad, lvl := data[o], data[o+1], data[o+2]
				if int(key) != r+1 || pad != 0 || int(lvl) != enh.EnchantLevel || lvl < 18 || lvl > 20 {
					return nil, fmt.Errorf("cronenchant: row %d group %d entry %d bad prefix [%d %d %d] (layout changed?)",
						r, g, e, key, pad, lvl)
				}
				total := int(bss.U32(data, o+3))
				if total < prevTotal {
					return nil, fmt.Errorf("cronenchant: row %d group %d entry %d non-monotonic total %d", r, g, e, total)
				}
				// stats become DSL effects (same shape as EnchantLevel.Effects);
				// funcs reuse the enhancement DSL vocabulary, see model.CaphrasLevel
				var effects []model.EffectDsl
				for k, fn := range caphrasEffectFuncs {
					v := bss.F32(data, o+7+4*k)
					if v > 0.1 {
						effects = append(effects, model.EffectDsl{
							Func: fn,
							Args: []float64{v},
						})
					}
				}
				enh.Steps = append(enh.Steps, model.CaphrasLevel{
					Level:       e + 1,
					Stones:      total - prevTotal,
					TotalStones: total,
					Effects:     model.FormatEffectFunctions(effects, false, "Caphras"),
				})
				prevTotal = total
				o += 39
			}
			cat.Levels = append(cat.Levels, enh)
		}
		if o != end {
			return nil, fmt.Errorf("cronenchant: row %d consumed %d of %d bytes", r, o-(8+r*rowSize), rowSize)
		}
		out = append(out, cat)
	}
	return out, nil
}
