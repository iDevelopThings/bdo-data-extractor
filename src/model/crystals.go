package model

// CrystalRules contains transfusion-family limits and special-slot restrictions.
type CrystalRules struct {
	Groups       []CrystalGroupRule       `json:"groups"`
	SpecialSlots []CrystalSpecialSlotRule `json:"specialSlots"`
}

// CrystalGroupRule limits how many crystals from one family may be equipped.
type CrystalGroupRule struct {
	Key        uint32 `json:"key"`
	Name       string `json:"name"`
	SourceName string `json:"sourceName,omitempty"`
	Max        int    `json:"max"`
}

// CrystalSpecialSlotRule lists crystal families accepted by a special slot.
type CrystalSpecialSlotRule struct {
	Slot          CrystalSpecialSlot `json:"slot"`
	AllowedGroups []uint32           `json:"allowedGroups"`
}
