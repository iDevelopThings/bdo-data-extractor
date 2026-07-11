package model

// ManufactureRecipe is one decoded manufacture.bss record. Output is intentionally
// absent (see file header): the real output is Group (server-resolved); the legacy
// direct-result slot is unreliable and dropped.
type ManufactureRecipe struct {
	Group   uint32       `json:"group"`   // ResultDropGroup / recipe group key (the output, server-resolved)
	Type    string       `json:"type"`    // action type (HEAT/ALCHEMY/COOK/…)
	Success float64      `json:"success"` // success rate, 0..1
	Inputs  []Ingredient `json:"inputs"`
}
