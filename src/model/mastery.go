package model

// MasteryBracket is one mastery threshold and its rate columns (each already /1e6).
// Cooking columns (order, from ToClient_getCookingStatStaticStatus): speedCookRate,
// basicMaxDropRate, addCriticalDropRate, addCriticalMaxDropRate, addRoyalTradeBonus.
// Alchemy has 9 columns (basicMaxDropRate, eventDropRate, per-tier event drop rates,
// addRoyalTradeBonus, …) whose exact roles are partly unconfirmed — stored faithfully.
type MasteryBracket struct {
	Mastery int       `json:"mastery"`
	Rates   []float64 `json:"rates"`
}

// ProcessingBracket is one processing-mastery threshold: the extra-output proc rate
// and the mass-process batch size (ToClient_getManufacturingStatCountRate).
type ProcessingBracket struct {
	Mastery  int     `json:"mastery"`
	ProcRate float64 `json:"procRate"`
	Batch    int     `json:"batch"`
}

// MasteryCurves is the decoded life-skill mastery → rate data.
type MasteryCurves struct {
	Cooking    []MasteryBracket    `json:"cooking"`
	Alchemy    []MasteryBracket    `json:"alchemy"`
	Processing []ProcessingBracket `json:"processing"`
}
