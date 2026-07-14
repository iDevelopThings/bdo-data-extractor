package model

// NPCSpawnType is one of the client SpawnType role flags from
// characterspawntype.dbss. Its numeric value is retained in JSON so the
// frontend can use the same icon/category mapping as the game.
type NPCSpawnType int

// IsMapRole reports whether the type is a specialized map/navigation role.
// Normal is excluded because it does not produce a service marker.
func (t NPCSpawnType) IsMapRole() bool {
	return t != NPCSpawnTypeNormal
}

// NPCSpawnTypes is an NPC's set of client-defined map/navigation roles.
type NPCSpawnTypes []NPCSpawnType

// Has reports whether the role set contains spawnType.
func (t NPCSpawnTypes) Has(spawnType NPCSpawnType) bool {
	for _, candidate := range t {
		if candidate == spawnType {
			return true
		}
	}
	return false
}

// HasMapRole reports whether the set contains a specialized map/navigation role.
func (t NPCSpawnTypes) HasMapRole() bool {
	for _, spawnType := range t {
		if spawnType.IsMapRole() {
			return true
		}
	}
	return false
}

const (
	// NPCSpawnTypeNormal is a normal NPC without a specialized service role.
	NPCSpawnTypeNormal NPCSpawnType = 0
	// NPCSpawnTypeSkillTrainer is a skill instructor.
	NPCSpawnTypeSkillTrainer NPCSpawnType = 1
	// NPCSpawnTypeItemRepairer repairs equipment.
	NPCSpawnTypeItemRepairer NPCSpawnType = 2
	// NPCSpawnTypeShopMerchant is a general shop merchant.
	NPCSpawnTypeShopMerchant NPCSpawnType = 3
	// NPCSpawnTypeImportantNPC is an important named NPC.
	NPCSpawnTypeImportantNPC NPCSpawnType = 4
	// NPCSpawnTypeTradeMerchant is a trade manager or merchant.
	NPCSpawnTypeTradeMerchant NPCSpawnType = 5
	// NPCSpawnTypeWarehouse is a storage keeper.
	NPCSpawnTypeWarehouse NPCSpawnType = 6
	// NPCSpawnTypeStable is a stable keeper.
	NPCSpawnTypeStable NPCSpawnType = 7
	// NPCSpawnTypeWharf is a wharf manager.
	NPCSpawnTypeWharf NPCSpawnType = 8
	// NPCSpawnTypeTransfer handles transport services.
	NPCSpawnTypeTransfer NPCSpawnType = 9
	// NPCSpawnTypeIntimacy participates in the amity system.
	NPCSpawnTypeIntimacy NPCSpawnType = 10
	// NPCSpawnTypeGuild provides guild services.
	NPCSpawnTypeGuild NPCSpawnType = 11
	// NPCSpawnTypeExplorer is a node manager.
	NPCSpawnTypeExplorer NPCSpawnType = 12
	// NPCSpawnTypeInn is an innkeeper.
	NPCSpawnTypeInn NPCSpawnType = 13
	// NPCSpawnTypeAuction provides auction services.
	NPCSpawnTypeAuction NPCSpawnType = 14
	// NPCSpawnTypeMating provides mount breeding services.
	NPCSpawnTypeMating NPCSpawnType = 15
	// NPCSpawnTypePotion sells potions.
	NPCSpawnTypePotion NPCSpawnType = 16
	// NPCSpawnTypeWeapon sells weapons.
	NPCSpawnTypeWeapon NPCSpawnType = 17
	// NPCSpawnTypeJewel sells crystals or jewelry.
	NPCSpawnTypeJewel NPCSpawnType = 18
	// NPCSpawnTypeFurniture sells furniture.
	NPCSpawnTypeFurniture NPCSpawnType = 19
	// NPCSpawnTypeCollect sells gathering supplies.
	NPCSpawnTypeCollect NPCSpawnType = 20
	// NPCSpawnTypeFish provides fishing services or supplies.
	NPCSpawnTypeFish NPCSpawnType = 21
	// NPCSpawnTypeWorker is a work supervisor.
	NPCSpawnTypeWorker NPCSpawnType = 22
	// NPCSpawnTypeAlchemy provides alchemy services.
	NPCSpawnTypeAlchemy NPCSpawnType = 23
	// NPCSpawnTypeGuildShop is a guild shop merchant.
	NPCSpawnTypeGuildShop NPCSpawnType = 24
	// NPCSpawnTypeItemMarket is a Central Market director.
	NPCSpawnTypeItemMarket NPCSpawnType = 25
	// NPCSpawnTypeTerritorySupply handles imperial supply delivery.
	NPCSpawnTypeTerritorySupply NPCSpawnType = 26
	// NPCSpawnTypeTerritoryTrade handles imperial trade delivery.
	NPCSpawnTypeTerritoryTrade NPCSpawnType = 27
	// NPCSpawnTypeSmuggle is a smuggler.
	NPCSpawnTypeSmuggle NPCSpawnType = 28
	// NPCSpawnTypeCook provides cooking services.
	NPCSpawnTypeCook NPCSpawnType = 29
	// NPCSpawnTypePC marks a player-character navigation category.
	NPCSpawnTypePC NPCSpawnType = 30
	// NPCSpawnTypeGrocery is a food or grocery merchant.
	NPCSpawnTypeGrocery NPCSpawnType = 31
	// NPCSpawnTypeRandomShop is a random shop merchant.
	NPCSpawnTypeRandomShop NPCSpawnType = 32
	// NPCSpawnTypeSupplyShop is a general supply merchant.
	NPCSpawnTypeSupplyShop NPCSpawnType = 33
	// NPCSpawnTypeRandomShopDay is a daytime random shop merchant.
	NPCSpawnTypeRandomShopDay NPCSpawnType = 34
	// NPCSpawnTypeFishSupplyShop is a fishing-supply merchant.
	NPCSpawnTypeFishSupplyShop NPCSpawnType = 35
	// NPCSpawnTypeGuildSupplyShop is a guild-supply merchant.
	NPCSpawnTypeGuildSupplyShop NPCSpawnType = 36
	// NPCSpawnTypeGuildStable is a guild stable keeper.
	NPCSpawnTypeGuildStable NPCSpawnType = 37
	// NPCSpawnTypeGuildWharf is a guild wharf manager.
	NPCSpawnTypeGuildWharf NPCSpawnType = 38
	// NPCSpawnTypePCRoomStable is an internet-cafe stable keeper.
	NPCSpawnTypePCRoomStable NPCSpawnType = 39
	// NPCSpawnTypeInstrument is an instrument merchant.
	NPCSpawnTypeInstrument NPCSpawnType = 40
	// NPCSpawnTypeUnknown41 is used by current client data but omitted from the
	// shipped CppEnums.SpawnType Lua table.
	NPCSpawnTypeUnknown41 NPCSpawnType = 41
	// NPCSpawnTypeTrainingVehicleShop is a training-vehicle merchant.
	NPCSpawnTypeTrainingVehicleShop NPCSpawnType = 42
	// NPCSpawnTypeAbyssOneEnterPositionGuide guides players to the Magnus.
	NPCSpawnTypeAbyssOneEnterPositionGuide NPCSpawnType = 43
	// NPCSpawnTypeChangeMarniStone exchanges Marni's Stones.
	NPCSpawnTypeChangeMarniStone NPCSpawnType = 44
	// NPCSpawnTypeChurchBuff provides church buffs.
	NPCSpawnTypeChurchBuff NPCSpawnType = 45
)
