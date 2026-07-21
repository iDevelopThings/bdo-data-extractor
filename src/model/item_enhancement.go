package model

// UnlocksDawnCrystalSlot reports whether equipment of this enhancement type
// satisfies the crystal-preset UI's isKarazardAccessory predicate.
func (t ItemEnhancementType) UnlocksDawnCrystalSlot() bool {
	return t == ItemEnhancementTypeKharazadAccessory || t == ItemEnhancementTypePreonneAccessory
}
