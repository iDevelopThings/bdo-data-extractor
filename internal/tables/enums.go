package tables

import (
	"math/bits"
	"sort"
)

// Enum maps transcribed from some private server sources. Used to turn the raw
// byte enum fields in an item row into human-readable names.
//
// Note: the central-market category names (item bytes @188/@189) are NOT here —
// they come straight from the client loc table 44 (see loc.LoadGame).

var gradeNames = map[byte]string{0: "white", 1: "green", 2: "blue", 3: "yellow", 4: "red", 5: "purple"}

// classBitNames maps a bit of the item class-restriction mask (itemenchant @77) to
// the class that can use the item. The mask is a u64 — newer classes live in the high
// dword (32 Seraph / 33 Dosa / 34 Deadeye), which is why reading it as a u32 silently
// dropped them. The mask-bit order is its own enum (derived from class-tagged costume
// data), maintained by hand whenever a new class ships — given that, we use our own
// names rather than sourcing them from loc. Bits 13/14/18/22 and 35/36 are unused/
// reserved class slots. Class-locked gear sets only its own class bit(s); all-class
// gear sets most bits, but not by a fixed value — older items predate Seraph/Dosa/
// Deadeye and set fewer bits — so ClassRestriction tests by popcount, not equality.
var classBitNames = map[int]string{
	0: "Warrior", 1: "Hashashin", 2: "Sage", 3: "Wukong", 4: "Ranger",
	5: "Guardian", 6: "Scholar", 7: "Drakania", 8: "Sorceress", 9: "Nova",
	10: "Corsair", 11: "Lahn", 12: "Berserker", 15: "Maegu", 16: "Tamer",
	17: "Shai", 19: "Striker", 20: "Musa", 21: "Maehwa", 23: "Mystic",
	24: "Valkyrie", 25: "Kunoichi", 26: "Ninja", 27: "Dark Knight", 28: "Wizard",
	29: "Archer", 30: "Woosa", 31: "Witch", 32: "Seraph", 33: "Dosa", 34: "Deadeye",
}

// allClassMask is every known class bit OR'd together; allClassCount is how many.
var allClassMask, allClassCount = func() (uint64, int) {
	var m uint64
	for b := range classBitNames {
		m |= uint64(1) << uint(b)
	}
	return m, bits.OnesCount64(m)
}()

// ClassRestriction resolves a class-restriction mask (u64 @77) to the class names
// allowed, ordered by class bit. Returns nil when unrestricted: no class bits, or a
// broad set (≥ half the classes) — all-class gear, whose exact bit set varies by the
// item's era, so we test by popcount rather than a fixed all-class value.
func ClassRestriction(mask uint64) []string {
	known := mask & allClassMask
	n := bits.OnesCount64(known)
	if n == 0 || n*2 >= allClassCount {
		return nil
	}
	bitsSet := make([]int, 0, 4)
	for b := range classBitNames {
		if known&(uint64(1)<<uint(b)) != 0 {
			bitsSet = append(bitsSet, b)
		}
	}
	sort.Ints(bitsSet)
	out := make([]string, len(bitsSet))
	for i, b := range bitsSet {
		out[i] = classBitNames[b]
	}
	return out
}

var itemTypeNames = map[byte]string{
	0:  "Normal",
	1:  "Equip",
	2:  "Skill",
	3:  "Tent",
	4:  "Installation",
	5:  "Jewel",
	6:  "CannonBall",
	7:  "Mapae",
	8:  "Material",
	9:  "Interaction",
	10: "ContentsEvent",
	11: "ToVehicle",
}

// EItemClassify — the main item category.
var classifyNames = map[byte]string{
	0:  "Etc",
	1:  "MainWeapon",
	2:  "SubWeapon",
	3:  "Armor",
	4:  "Accessory",
	5:  "BlackStone",
	6:  "Jewel",
	7:  "Potion",
	8:  "Cook",
	9:  "PearlGoods",
	10: "Housing",
	11: "Vehicle",
	12: "Mine",
	13: "Wood",
	14: "Seed",
	15: "Leather",
	16: "Fish",
	17: "DyeAmpule",
	18: "SpecialGoods", // currencies (Silver/Pearl/Loyalties/Crow Coin), loot boxes, titles, coupons, seals — 16k items; name from client enum eItemClassify_SpecialGoods
}

// equipTypeNames maps the EquipType byte (@7) to the item's specific type, using
// the client's Central Market sub-category display names (verified against the
// market data of tradeable items). @7 is the finest client-side item-type field
// and is present on ALL equipment — tradeable or not — so (Category @5 + this)
// gives a market-style "Main Weapon > Longsword" taxonomy even for items that
// never appear on the market (Tuvala/boss gear/etc.). @7=57 (Awakening Weapon) is
// the one value shared across all awakening weapons; the market splits those by
// name, which @7 cannot. A few slot values are reused by ship/mount gear (a ship
// Totem carries the Helmet type); the name here is the character-equipment meaning.
var equipTypeNames = map[byte]string{
	0: "None",
	// main weapons (one type per class)
	1: "Longsword", 2: "Shortsword", 3: "Blade", 6: "Staff", 28: "Amulet",
	29: "Axe", 31: "Longbow", 63: "Kriegsmesser", 65: "Gauntlet",
	67: "Crescent Pendulum", 78: "Florang", 80: "Shamshir", 83: "Battle Axe",
	86: "Morning Star", 88: "Kyve", 89: "Serenaca", 93: "Slayer",
	96: "Foxspirit Charm", 97: "Swallowtail Fan", 100: "Hammers", 109: "Hwando",
	112: "Revolvers", 114: "Power Pole", 116: "Sacramentum",
	// sub-weapons (one type per class)
	8: "Shield", 32: "Dagger", 33: "Talisman", 34: "Ornamental Knot",
	36: "Horn Bow", 37: "Trinket", 55: "Kunai", 56: "Shuriken", 66: "Vambrace",
	70: "Noble Sword", 73: "Crossbow", 74: "Ra'ghon", 79: "Vitclari",
	81: "Haladie", 87: "Quoratum", 90: "Mareca", 94: "Shard", 98: "Binyeo Knife",
	99: "Do Stave", 108: "Gravity Cores", 110: "Gombangdae", 113: "Shotgun",
	115: "Gourd Bottle", 117: "Clavis",
	// awakening weapons (all share this value)
	57: "Awakening Weapon",
	// armor
	9: "Armor", 11: "Gloves", 12: "Shoes", 13: "Helmet",
	// accessories
	15: "Necklace", 16: "Ring", 17: "Earring", 18: "Belt", 92: "Artifact",
	// costumes / appearance
	22: "Costume Armor", 24: "Costume Gloves", 25: "Costume Shoes",
	26: "Costume Helmet", 30: "Costume Main Weapon", 35: "Costume Sub-weapon",
	58: "Costume Awakening Weapon", 38: "Underwear", 76: "Swimsuit",
	39: "Costume Earring", 40: "Costume Eyewear", 41: "Costume Piercing",
	52: "Wagon Cover",
	// life / gathering tools
	44: "Fishing Rod", 46: "Gathering Tool", 48: "Fishing Harpoon", 59: "Float",
	102: "Lumbering Axe", 103: "Fluid Collector", 104: "Hoe", 105: "Butcher Knife",
	106: "Tanning Knife", 107: "Pickaxe", 111: "Gathering Carrier",
	// alchemy stone / misc equip
	54: "Alchemy Stone", 19: "Lantern", 72: "Tome", 51: "Ship Decoration",
	5: "Store",
	// rare hunting / instruments / event (a handful of items each)
	43: "Matchlock", 53: "Matchlock", 64: "Matchlock", 77: "Sniper Rifle",
	45: "Flute", 47: "Cane", 49: "Net", 50: "Drum", 60: "Cymbals", 61: "Guitar",
	62: "Trumpet", 68: "Ammunition", 69: "Hammer", 71: "Cannonball",
	75: "Water Balloon",
}

// slotNames maps the equip-slot enum (itemenchant byte @14 = getEquipSlotNo) to a
// friendly slot name. The enum itself is the client's __eEquipSlotNo (the equipment
// window's physical slots); the labels here are ours. Unlike equipTypeNames (@7,
// the weapon class, blank for artifacts/life-tools), this is the only slot source
// for artifacts, life/gathering tools and costume accessories. The trailing comment
// on each line is the client __eEquipSlotNo* constant it corresponds to. A handful
// of slot numbers are reused across ship/mount contexts; the label is the dominant
// character-equipment meaning.
var slotNames = map[byte]string{
	0:  "Main Weapon",               // RightHand
	1:  "Sub-weapon",                // LeftHand
	2:  "Fishing Chair",             // (life-tool seat; GatheringTools)
	3:  "Armor",                     // Chest
	4:  "Gloves",                    // Glove
	5:  "Shoes",                     // Boots
	6:  "Helmet",                    // Helm
	7:  "Necklace",                  // Necklace
	8:  "Ring",                      // Ring1
	10: "Earring",                   // Earing1
	12: "Belt",                      // Belt
	13: "Lantern",                   // Lantern
	14: "Costume: Armor",            // AvatarChest
	15: "Costume: Gloves",           // AvatarGlove
	16: "Costume: Shoes",            // AvatarBoots
	17: "Costume: Helmet",           // AvatarHelm
	18: "Costume: Main Weapon",      // AvatarWeapon
	19: "Costume: Sub-weapon",       // AvatarSubWeapon
	20: "Underwear",                 // AvatarUnderwear
	21: "Costume: Earring",          // FaceDecoration1
	22: "Costume: Headpiece",        // FaceDecoration2
	23: "Costume: Piercing",         // FaceDecoration3
	25: "Ship Gear",                 // (mount/ship equipment)
	26: "Ship Gear",                 // (mount/ship equipment)
	27: "Alchemy Stone",             // AlchemyStone
	29: "Awakening Weapon",          // AwakenWeapon
	30: "Costume: Awakening Weapon", // AvatarAwakenWeapon
	31: "Tome",                      // QuestBook
	32: "Artifact",                  // Artifact1
	34: "Lumbering Axe",             // Axe
	35: "Fluid Collector",           // Syringe
	36: "Hoe",                       // Hoe
	37: "Butcher Knife",             // ButcheryKnife
	38: "Tanning Knife",             // SkinKnife
	39: "Pickaxe",                   // PickAx
	40: "Fishing Rod",               // FishingRod
	41: "Fishing Float",             // Bobber
	42: "Fishing Harpoon",           // FishingHarpoon
	45: "Gathering Carrier",         // SubTool
}

// equipKindNames maps the coarse equip-kind enum (itemenchant byte @15) to the broad
// gear class. Only the two combat-stat kinds are distinct; everything else (costumes,
// accessories, tools, artifacts, …) falls under "Other".
var equipKindNames = map[byte]string{
	0: "Weapon",
	1: "Armor",
	2: "Other",
}

// manufactureActionNames — the manufacture.bss action type by index (the table's
// string-table order). These are the same process names the recipe XMLs use (minus
// the "MANUFACTURE_" prefix); manufacture.bss keys them by this index.
var manufactureActionNames = []string{
	"HEAT", "ALCHEMY", "DRY", "GRIND", "CRAFT", "COOK",
	"SHAKE", "ROYALGIFT_ALCHEMY", "GUILD", "ROYALGIFT_COOK", "THINNING", "FIREWOOD",
}

// manufactureActionName returns the action name for an index, or "" if out of range.
func manufactureActionName(i uint32) string {
	if i < uint32(len(manufactureActionNames)) {
		return manufactureActionNames[i]
	}

	return ""
}

func name(m map[byte]string, v byte) string {
	if s, ok := m[v]; ok {
		return s
	}
	return ""
}
