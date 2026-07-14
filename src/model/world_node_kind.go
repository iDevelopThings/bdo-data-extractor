package model

// WorldNodeKind is the exploration.bss worldmap node kind at record offset 5.
// Its numeric value is retained in JSON for compatibility with the client data.
type WorldNodeKind uint8

const (
	// WorldNodeKindNormal is a generic field, location, island, or connecting node.
	WorldNodeKindNormal WorldNodeKind = 0
	// WorldNodeKindVillage is a town, village, settlement, or minor hub.
	WorldNodeKindVillage WorldNodeKind = 1
	// WorldNodeKindCity is a major city or capital.
	WorldNodeKindCity WorldNodeKind = 2
	// WorldNodeKindGate is a gateway, outpost, fort, or guard camp.
	WorldNodeKindGate WorldNodeKind = 3
	// WorldNodeKindFarm is a crop production sub-node.
	WorldNodeKindFarm WorldNodeKind = 4
	// WorldNodeKindTrade is a farm, ranch, or resource-camp main node.
	WorldNodeKindTrade WorldNodeKind = 5
	// WorldNodeKindCollect is a gathering production sub-node.
	WorldNodeKindCollect WorldNodeKind = 6
	// WorldNodeKindQuarry is a mining production sub-node.
	WorldNodeKindQuarry WorldNodeKind = 7
	// WorldNodeKindLogging is a lumbering production sub-node.
	WorldNodeKindLogging WorldNodeKind = 8
	// WorldNodeKindDangerous is a dangerous or combat-site main node.
	WorldNodeKindDangerous WorldNodeKind = 9
	// WorldNodeKindFinance is a town asset-management service node.
	WorldNodeKindFinance WorldNodeKind = 10
	// WorldNodeKindFishTrap is a fish-drying production sub-node.
	WorldNodeKindFishTrap WorldNodeKind = 11
	// WorldNodeKindMinorFinance is a worker investment-bank production sub-node.
	WorldNodeKindMinorFinance WorldNodeKind = 12
	// WorldNodeKindMonopolyFarm is a specialty production sub-node.
	WorldNodeKindMonopolyFarm WorldNodeKind = 13
	// WorldNodeKindCraft is an animal-product or other crafting production sub-node.
	WorldNodeKindCraft WorldNodeKind = 14
	// WorldNodeKindExcavation is an excavation or special-workshop production sub-node.
	WorldNodeKindExcavation WorldNodeKind = 15
)
