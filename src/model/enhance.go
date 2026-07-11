package model

import "strconv"

// enhanceTiers are the roman enhancement tiers, PRI (1st) through DEC (10th).
var enhanceTiers = [...]string{
	"PRI (I)", "DUO (II)", "TRI (III)", "TET (IV)", "PEN (V)",
	"HEX (VI)", "SEP (VII)", "OCT (VIII)", "NOV (IX)", "DEC (X)",
}

// EnhanceLevelName is the display name of an enhancement level: 0 = "Base", levels
// below romanStart are "+N", and romanStart upward are the roman tiers. romanStart is
// 16 for gear that runs the numeric "+1..+15" phase before the roman tiers, or 1 for
// accessories / roman-from-1 lines (which enhance PRI→PEN/DEC directly).
func EnhanceLevelName(level, romanStart int) string {
	switch {
	case level <= 0:
		return "Base"
	case level < romanStart:
		return "+" + strconv.Itoa(level)
	default:
		if i := level - romanStart; i >= 0 && i < len(enhanceTiers) {
			return enhanceTiers[i]
		}
		return "UNKNOWN"
	}
}
