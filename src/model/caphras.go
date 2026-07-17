package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// CaphrasCategory is one Caphras-enhancement cost/stat chart from
// cronenchant.bss ("CronEnchant" is the game's internal name for the Caphras
// system). Items map to a category via the client's computed cronKey (the
// mapping itself is not stored client-side); the chart is shared by every item
// of the category. Key 1..10; categories with identical charts exist (1/2,
// 7/8) — the game distinguishes them by slot kind.
type CaphrasCategory struct {
	*models.BaseFor[CaphrasCategory]

	Key    int                  `json:"key"`
	Levels []CaphrasEnhancement `json:"levels"`
}

// CaphrasEnhancement is a category's chart for one enhancement level
// (18 = TRI, 19 = TET, 20 = PEN — the only levels Caphras applies to).
type CaphrasEnhancement struct {
	EnchantLevel int            `json:"enchantLevel"`
	Steps        []CaphrasLevel `json:"steps"`
}

// CaphrasLevel is one Caphras level step: Stones is the cost of this step
// (from the previous level), TotalStones the cumulative cost to reach it (the
// value the table stores), and Effects the total added stats while at it, in
// the same DSL shape as EnchantLevel.Effects so one renderer computes both.
//
// The func names reuse the enhancement DSL vocabulary where the game has one:
// ALL_AP_INCRE (AP), ALL_HIT_INCRE (accuracy), ALL_EVA_INCRE (evasion),
// ALL_DAM_REDUCE_INCRE (damage reduction), HP_UP, MP_WP_SP_UP. The two hidden
// stats never appear in the game's DSL, so they use extension names following
// the same pattern: HIDDEN_EVA_INCRE, HIDDEN_DAM_REDUCE_INCRE (in-game they
// display as a second All Evasion / Damage Reduction line).
type CaphrasLevel struct {
	Level       int           `json:"level"`
	Stones      int           `json:"stones"`
	TotalStones int           `json:"totalStones"`
	Stats       CaphrasStats  `json:"stats"`
	Effects     []EffectGroup `json:"effects,omitempty"`
}

// CaphrasStats is the eight-column cumulative stat block stored for one
// Caphras level.
type CaphrasStats struct {
	AP                    float64 `json:"ap,omitempty"`
	Accuracy              float64 `json:"accuracy,omitempty"`
	Evasion               float64 `json:"evasion,omitempty"`
	HiddenEvasion         float64 `json:"hiddenEvasion,omitempty"`
	DamageReduction       float64 `json:"damageReduction,omitempty"`
	HiddenDamageReduction float64 `json:"hiddenDamageReduction,omitempty"`
	MaxHP                 float64 `json:"maxHp,omitempty"`
	MaxMP                 int     `json:"maxMp,omitempty"`
}
