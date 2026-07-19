package model

// TooltipTitle returns the client tooltip's top-right item-type label.
func (t ItemType) TooltipTitle(forTrade bool) string {
	if !forTrade {
		return t.Title()
	}
	if t == ItemTypeSkill {
		return "Trade Item"
	}
	return t.Title() + "/Trade Item"
}

// TooltipTypeTitle returns the client tooltip's top-right item-type label.
func (i Item) TooltipTypeTitle() string {
	return i.ItemType.TooltipTitle(i.ForTrade)
}
