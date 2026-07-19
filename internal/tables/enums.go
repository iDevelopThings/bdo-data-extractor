package tables

import (
	"fmt"
	"math/bits"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// classBitTypes maps a bit of the item class-restriction mask (itemenchant @77) to
// the class that can use the item. The mask is a u64 — newer classes live in the high
// dword (32 Seraph / 33 Dosa / 34 Deadeye), which is why reading it as a u32 silently
// dropped them. The mask-bit order is its own enum (derived from class-tagged costume
// data), maintained by hand whenever a new class ships — given that, we use our own
// names rather than sourcing them from loc. Bits 13/14/18/22 and 35/36 are unused/
// reserved class slots. Class-locked gear sets only its own class bit(s); all-class
// gear sets most bits, but not by a fixed value — older items predate Seraph/Dosa/
// Deadeye and set fewer bits — so ClassRestriction tests by popcount, not equality.
var classBitTypes = func() map[int]model.CharacterClassType {
	types := make(map[int]model.CharacterClassType)
	for value := 0; value <= int(model.CharacterClassTypeReserved36); value++ {
		classType := model.CharacterClassType(value)
		if !classType.Reserved() {
			types[value] = classType
		}
	}
	return types
}()

// allClassMask is every known class bit OR'd together; allClassCount is how many.
var allClassMask, allClassCount = func() (uint64, int) {
	var m uint64
	for b := range classBitTypes {
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
	for b := range classBitTypes {
		if known&(uint64(1)<<uint(b)) != 0 {
			bitsSet = append(bitsSet, b)
		}
	}
	sort.Ints(bitsSet)
	out := make([]string, len(bitsSet))
	for i, b := range bitsSet {
		out[i] = classBitTypes[b].String()
	}
	return out
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

// equipKindNames maps the coarse equip-kind enum (itemenchant byte @15) to the broad
// gear class. Kind 0 includes combat weapons and weapon-like life/hunting tools;
// kind 1 is combat armor; costumes, accessories and artifacts use kind 2.
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
	return fmt.Sprintf("Unknown%d", v)
}
