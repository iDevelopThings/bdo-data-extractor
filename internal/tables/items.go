// Package tables decodes specific BDO data tables into structured values using
// the byte layouts reverse-engineered for this client. Each decoder takes the
// already-decoded table bytes (and its offset index where applicable) and does
// a single linear pass — no per-row re-parsing of the whole record.
package tables

import (
	"bytes"
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// enchant-entry rows in itemenchant use very large internal keys (~3e8+);
// real public item ids are well under this. Used to skip non-item rows.
const maxItemID = 10_000_000

// ItemStat holds the static fields decoded from an itemenchant.dbss item row.
type ItemStat struct {
	ItemType      string   // EItemType         (byte @4)
	Category      string   // EItemClassify     (byte @5)
	Grade         string   // EItemGradeType    (byte @6)
	EquipType     string   // EquipType / slot  (byte @7)
	MarketCatID   byte     // central-market main category id (byte @188); named via loc table 44
	MarketSubID   byte     // central-market sub category id  (byte @189)
	SkillKey      uint32   // skill key (u32 @192) -> skill.dbss -> buff effect chain
	Weight        float64  // int32 @63, value/10000 (LT)
	Buy           int64    // OriginalPrice, int64 @110
	Sell          int64    // SellPriceToNpc, int64 @118
	Repair        int64    // RepairPrice, int32 @126
	MaxDurability int      // u16 @185, equipment only (0x7FFF/large = no-durability sentinel)
	Classes       []string // usable classes (mask @77), nil = all classes / unrestricted
	Slot          string   // normalized equip slot (byte @14), equipment only
	Kind          string   // coarse equip kind: Weapon/Armor/Other (byte @15), equipment only
	ExtraSlots    []string // additional occupied slots (@16-18), multi-slot costumes only
	Expiration    int      // u32 @69, item expiration in MINUTES (0 = permanent)
	RequiredLevel int      // byte @97, required character level (0/1 = none)
	MaxStack      int64    // u32 @101, max stack size (sentinels = unlimited -> 0)
	DyeParts      int      // byte @160, dyeable part count
	JewelGroup    int      // u16 @ len-4 (record footer), crystal transfusion group; -1 = none
	Icon          string   // "New_Icon/....dds" embedded in the row

	// icon-end-anchored fields (a fixed ~59-byte block follows the icon string):
	Marketable          bool           // iconEnd+0 — may be listed on the central market
	BindType            model.BindType // iconEnd+15 — vestedType (see model.BindType constants)
	MarketRegisterLimit int64          // iconEnd+42 i64 — max units per market registration (gear 5, potions 500, mem frags 1000); huge sentinel/0 = no limit/none

	// Fields named by correlating decoded rows against a labeled reference
	// field dictionary (two-sided agreement ≥93% over the ~28k shared item ids
	// unless noted; see FORMATS §3 provenance). Comments give the field name.
	ItemMaterial  int   // @62 ItemMaterial — material/model family (99%); previously misread as NewEquipType
	Stackable     bool  // @67 IsStack
	ApplyDirectly bool  // @68 DoApplyDirectly
	VestedType    int   // @73 VestedType (classic bind enum; BindType/icon+15 is a separate field)
	UserVested    bool  // @74 IsUserVested
	ForTrade      bool  // @75 IsForTrade (trading-skill goods)
	TradeType     int   // @76 TradeType
	LifeExpType   int   // @105 LifeExpType
	EventType     int   // @134 ContentsEventType (u8; 171/0 = none -> 0)
	EventParam1   int64 // @136 ContentsEventParam1 (u32)
	EventParam2   int64 // @140 ContentsEventParam2 (u32)
	ShownInNote   bool  // @151 == 0 (inverse of HideFromNote, whose default is 1)
	Cash          bool  // @152 IsCash (pearl-shop item)
	CronEnchant   int   // @153 CronEnchantcontrol
	Dyeable       bool  // @156 IsDyeable
	PersonalTrade bool  // @184 IsPersonalTrade (player-to-player handable; 329 items)
	NodeFreeTrade bool  // @190 NodeFreeTrade

	FamilyInventory  bool // icon-end+13 == 2 — the Lua's checkPushFamilyInventory (can be stored in the account-wide Family Inventory; ~1,089 shareable consumables/materials)
	ContributionCost int  // NeedContribute — Contribution Points to rent/place (tail cpMarker+20; "[CP]" rental gear + placeable fences/tents; 0 = none)

	// Still-unidentified item-row bytes as typed, deviation-only fields (see
	// model.ItemUnknowns). A nil field holds its default; decodeItemRow reads
	// every byte and sets these directly — no map, so each is renamed in place
	// once identified.
	U model.ItemUnknowns
}

// cpMarker anchors the common tail property block that carries the Contribution
// Point cost. It appears in most rows (the block itself is generic); the cost
// lives 20 bytes past it and is 0 for non-CP items. See contributionCostOf.
var cpMarker = []byte{0x13, 0x06, 0x00, 0x00, 0x00, 0x00, 0x13}

// contributionCostOf extracts the Contribution Point rental/placement cost (the
// reference schema's NeedContribute) from the type-dependent tail. The value is
// a u32 20 bytes after cpMarker, searched from the icon-string end so the
// per-type tail-order variation before the block doesn't matter. The sane bound
// rejects rows whose first marker match is coincidental (those read huge/garbage
// there); validated to fire on exactly the 233 "[CP]"-named items — CP rental
// gear ([CP] Nesser/Kaia/Carnage weapons & armor) plus placeable fences/tents —
// with values 1..100, zero false positives.
func contributionCostOf(rec []byte, iconEnd int) int {
	if iconEnd < 0 || iconEnd >= len(rec) {
		return 0
	}
	m := bytes.Index(rec[iconEnd:], cpMarker)
	if m < 0 {
		return 0
	}
	m += iconEnd
	if m+24 > len(rec) {
		return 0
	}
	if v := int32(bss.U32(rec, m+20)); v >= 1 && v <= 1000 {
		return int(v)
	}
	return 0
}

// item-row field offsets (reverse-engineered, validated against the live client).
const (
	offItemType   = 4
	offClassify   = 5
	offGrade      = 6
	offEquipType  = 7
	offSlot       = 14 // normalized equip slot (equipment)
	offKind       = 15 // coarse equip kind: weapon/armor/other (equipment)
	offExtraSlot  = 16 // extra occupied slots @16..@18 (multi-slot costumes), 46 = none
	offWeight     = 63
	offExpiration = 69  // u32 expiration period in minutes (0 = permanent)
	offClassMask  = 77  // u64 class-restriction bitmask (equipment; high dword = newer classes)
	offReqLevel   = 97  // required character level (0/1 = none)
	offMaxStack   = 101 // u32 max stack size (0x7FFFFFFF / 0xFFFFFF00 = unlimited)
	offBuy        = 110
	offSell       = 118
	offRepair     = 126
	offDyeParts   = 160           // dyeable part count
	offDurability = 185           // u16 max durability (equipment)
	offMarketCat  = 188           // main category byte; @189 = sub category byte
	itemRowMin    = offRepair + 4 // smallest row we can fully read

	eItemTypeEquip = 1     // EItemType.Equip — only equipment carries durability
	slotNone       = 46    // the "no slot" filler value of the slot array
	noJewelGroup   = 65535 // footer group sentinel: not a socket crystal
)

var iconMarker = []byte("New_Icon")

func extractIcon(rec []byte) string {
	i := bytes.Index(rec, iconMarker)
	if i < 0 {
		return ""
	}
	j := bytes.Index(rec[i:], []byte(".dds"))
	if j < 0 {
		return ""
	}
	return string(rec[i : i+j+4])
}

// enchantKeyOf returns the item's EnchantKey (enchant curve base id): the u32
// immediately before the Name string, which sits just before the icon path. The
// Name is the inline UTF-16 string whose end abuts the icon's length prefix. This
// is deterministic — unlike scanning the pre-icon bytes for a plausible base id.
func enchantKeyOf(rec []byte) (uint32, bool) {
	ic := bytes.Index(rec, iconMarker)
	if ic < 16 {
		return 0, false
	}
	end := ic - 8 // the icon's i64 length prefix; the Name string ends here
	for nameLen := 1; nameLen <= 80; nameLen++ {
		no := end - 8 - nameLen*2 // candidate Name length-prefix offset
		if no < 4 {
			break
		}
		if int(int64(bss.U64(rec, no))) == nameLen {
			return bss.U32(rec, no-4), true
		}
	}
	return 0, false
}

// DecodeItemStats decodes every item row of itemenchant.dbss into an ItemStat,
// keyed by public item id — see decodeItemRow for the row read.
func DecodeItemStats(offsetRaw, data []byte) (map[uint32]ItemStat, error) {
	idx, err := bss.ParseOffsetIndex(offsetRaw, len(data))
	if err != nil {
		return nil, fmt.Errorf("itemenchant offset: %w", err)
	}
	out := make(map[uint32]ItemStat, len(idx))
	for _, e := range idx {
		if e.Key >= maxItemID { // skip enchant-entry rows
			continue
		}
		rec, ok := e.Slice(data)
		if !ok || len(rec) < itemRowMin || bss.U32(rec, 0) != e.Key {
			continue
		}
		out[e.Key] = decodeItemRow(rec, e.Key)
	}
	return out, nil
}

// decodeItemRow reads one itemenchant row as a straight sequential field stream.
// The FIXED header (@0-192) is read in order with a bss.Cursor — no
// offset-jumping. The name and the post-icon block are reached via the icon
// anchor (the icon-string end): the variable enchant block before the name
// isn't reliably sequential — ship-upgrade gear uses a different layout — so the
// anchor keeps those fields correct for every row. The tail beyond the post-icon
// block is item-type-specific and left alone except the Contribution cost (found
// by its marker). Every byte is read; unidentified ones land on typed
// deviation-only st.U.Unknown<off> fields (model.ItemUnknowns) rather than being
// skipped. Every row is >= 678 bytes, so no field is ever truncated.
func decodeItemRow(rec []byte, id uint32) ItemStat {
	var st ItemStat
	c := bss.NewCursor(rec, 0, len(rec))

	// --- fixed header, every byte read in order (no skipping) ---
	// Identified fields land on named struct fields; each unidentified byte is
	// read into its own st.U.Unknown<off> field (deviation-only). Constant/
	// reserved runs and the wide-distribution data blocks (@158-159, @162-167,
	// @170-175, @179-183) are still READ — via c.Bytes/c.U8 to advance the
	// cursor — but not surfaced, since they hold no per-item signal worth a field.
	c.U32()              // @0    id (== key)
	itemType := c.Byte() // @4
	st.ItemType = name(itemTypeNames, itemType)
	st.Category = name(classifyNames, c.Byte())   // @5
	st.Grade = name(gradeNames, c.Byte())         // @6
	st.EquipType = name(equipTypeNames, c.Byte()) // @7
	st.U.Unknown8 = dev(c.U8(), 0)                // @8    flag cluster
	st.U.Unknown9 = dev(c.U8(), 0)                // @9
	st.U.Unknown10 = dev(c.U8(), 0)               // @10
	st.U.Unknown11 = dev(c.U8(), 0)               // @11   set on 26,406 items
	st.U.Unknown12 = dev(c.U8(), 2)               // @12   default 2
	st.U.Unknown13 = dev(c.U8(), 0)               // @13
	slot := c.Byte()                              // @14
	kind := c.Byte()                              // @15
	extra := c.U8N(46)                            // @16-61  occupied-slot list (46 = none)
	st.ItemMaterial = int(c.U8())                 // @62
	st.Weight = float64(c.I32()) / 10000.0        // @63
	st.Stackable = c.Bool()                       // @67
	st.ApplyDirectly = c.Bool()                   // @68
	st.Expiration = int(c.U32())                  // @69
	st.VestedType = int(c.U8())                   // @73
	st.UserVested = c.Bool()                      // @74
	st.ForTrade = c.Bool()                        // @75
	st.TradeType = int(c.U8())                    // @76
	classMask := c.U64()                          // @77
	st.U.Unknown85 = dev(int(c.U32()), 0)         // @85-88  u32 bitfield (item property flags)
	c.U32()                                       // @89-92  reserved (always 0; maybe @85's u64 high dword)
	st.U.Unknown93 = dev(c.U8(), 0)               // @93     (== @98 on all 105 items that set it)
	c.Bytes(3)                                    // @94-96  constant 0
	reqLevel := c.U8()                            // @97
	st.U.Unknown98 = dev(c.U8(), 0)               // @98
	st.U.Unknown99 = dev(c.U8(), 0)               // @99
	st.U.Unknown100 = dev(c.U8(), 0)              // @100
	maxStack := c.U32()                           // @101
	st.LifeExpType = int(c.U8())                  // @105
	st.U.Unknown106 = dev(c.U8(), 0)              // @106
	st.U.Unknown107 = dev(c.U8(), 0)              // @107
	st.U.Unknown108 = dev(c.U8(), 0)              // @108
	c.U8()                                        // @109  constant 0
	st.Buy = c.I64()                              // @110
	st.Sell = c.I64()                             // @118
	st.Repair = int64(c.I32())                    // @126
	c.Bytes(4)                                    // @130-133  constant 0
	eventType := c.U8()                           // @134
	st.U.Unknown135 = dev(c.U8(), 0)              // @135
	eventP1, eventP2 := c.U32(), c.U32()          // @136, @140
	st.U.Unknown144 = dev(int(c.U16()), 0)        // @144-145 (u16)
	st.U.Unknown146 = dev(c.U8(), 0)              // @146
	c.U8()                                        // @147  constant 0
	st.U.Unknown148 = dev(c.U8(), 0)              // @148
	st.U.Unknown149 = dev(c.U8(), 255)            // @149  default 255
	st.U.Unknown150 = dev(c.U8(), 255)            // @150  default 255
	hideFromNote := c.U8()                        // @151
	st.Cash = c.Bool()                            // @152
	st.CronEnchant = int(c.U8())                  // @153
	st.U.Unknown154 = dev(c.U8(), 0)              // @154
	st.U.Unknown155 = dev(c.U8(), 0)              // @155
	st.Dyeable = c.Bool()                         // @156
	st.U.Unknown157 = dev(c.U8(), 1)              // @157  default 1
	c.Bytes(2)                                    // @158-159  data block (read-through; seqtail)
	dyeParts := c.U8()                            // @160
	st.U.Unknown161 = dev(c.U8(), 0)              // @161
	c.Bytes(6)                                    // @162-167  @164-165 data block + const (read-through)
	st.U.Unknown168 = dev(c.U8(), 0)              // @168  paired with @176 (same 16,655 items)
	st.U.Unknown169 = dev(c.U8(), 0)              // @169
	c.Bytes(6)                                    // @170-175  constant 0
	st.U.Unknown176 = dev(c.U8(), 0)              // @176  paired with @168
	st.U.Unknown177 = dev(c.U8(), 0)              // @177
	st.U.Unknown178 = dev(c.U8(), 0)              // @178
	c.Bytes(5)                                    // @179-183  constant 0
	st.PersonalTrade = c.Bool()                   // @184
	maxDur := c.U16()                             // @185
	st.U.Unknown187 = dev(c.U8(), 0)              // @187
	st.MarketCatID = c.Byte()                     // @188
	st.MarketSubID = c.Byte()                     // @189
	st.NodeFreeTrade = c.Bool()                   // @190
	st.U.Unknown191 = dev(c.U8(), 1)              // @191  default 1
	st.SkillKey = c.U32()                         // @192

	// header-derived
	st.ShownInNote = hideFromNote == 0
	if eventType != 0 && eventType != 171 { // 171 = the "no event" default
		st.EventType, st.EventParam1, st.EventParam2 = int(eventType), int64(eventP1), int64(eventP2)
	}
	if reqLevel > 1 && reqLevel <= 100 {
		st.RequiredLevel = int(reqLevel)
	}
	if maxStack > 0 && maxStack != 0x7FFFFFFF && maxStack != 0xFFFFFF00 { // sentinels = unlimited
		st.MaxStack = int64(maxStack)
	}
	if dyeParts > 0 && dyeParts <= 30 {
		st.DyeParts = int(dyeParts)
	}
	if itemType == eItemTypeEquip {
		st.Classes = ClassRestriction(classMask)
		st.Slot = name(slotNames, slot)
		st.Kind = name(equipKindNames, kind)
		for i := 0; i < 3; i++ { // extra occupied slots @16-18
			if extra[i] != slotNone {
				if s := name(slotNames, extra[i]); s != "" {
					st.ExtraSlots = append(st.ExtraSlots, s)
				}
			}
		}
		if d := int(maxDur); d > 0 && d < 10_000 { // large = no-durability sentinel
			st.MaxDurability = d
		}
	}

	// --- name + post-icon block, reached via the icon anchor ---
	st.Icon = extractIcon(rec)
	icEnd := -1
	if ic := bytes.Index(rec, iconMarker); ic >= 16 {
		icEnd = ic - 8 + 8 + int(int64(bss.U64(rec, ic-8)))
		if icEnd >= ic && icEnd+50 <= len(rec) {
			p := bss.NewCursor(rec, icEnd, len(rec))
			st.Marketable = p.Bool()             // +0
			st.U.UnknownIcon1 = dev(p.U8(), 0)   // +1
			st.U.UnknownIcon2 = dev(p.U8(), 9)   // +2   default 9 (currencies deviate)
			st.U.UnknownIcon3 = dev(p.U8(), 0)   // +3
			p.Bytes(4)                           // +4..+7  constant 0
			st.U.UnknownIcon8 = dev(p.U8(), 2)   // +8    Cook-product flag (best guess)
			st.U.UnknownIcon9 = dev(p.U8(), 0)   // +9
			st.U.UnknownIcon10 = dev(p.U8(), 0)  // +10   food tier (best guess)
			p.Bytes(2)                           // +11..+12  constant 0
			st.FamilyInventory = p.Bool()        // +13   checkPushFamilyInventory
			st.U.UnknownIcon14 = dev(p.U8(), 0)  // +14
			st.BindType = model.BindType(p.U8()) // +15   vestedType
			st.U.UnknownIcon16 = dev(p.U8(), 1)  // +16   resource class (best guess)
			// +17..+41 is a wide-distribution appearance/data block (like +46..+58):
			// read every byte, but only +19/+27 carry enum-ish values worth a field
			// — the rest is inspected via seqtail.
			p.Bytes(2)                                  // +17..+18
			st.U.UnknownIcon19 = dev(p.U8(), 0)         // +19
			p.Bytes(7)                                  // +20..+26
			st.U.UnknownIcon27 = dev(p.U8(), 0)         // +27
			p.Bytes(14)                                 // +28..+41  data block (read-through)
			if lim := p.I64(); lim > 0 && lim < 1<<32 { // +42  MarketRegisterLimit (huge = no limit)
				st.MarketRegisterLimit = lim
			}
			st.ContributionCost = contributionCostOf(rec, icEnd)
		} else {
			icEnd = -1
		}
	}

	// --- footer: crystal transfusion group (self-id echo then u16 group) ---
	st.JewelGroup = -1
	if l := len(rec); l >= 12 && bss.U32(rec, l-8) == id {
		if g := bss.U16(rec, l-4); g != noJewelGroup {
			st.JewelGroup = int(g)
		}
	}
	return st
}

// DecodeMaxLevels reads itemmaxlevel.dbss (records: [u32 id][u8 maxLevel]).
// Its offset index is (key=item id, offset, 0) so the generic detector can't be
// used; we read the two known columns directly.
func DecodeMaxLevels(offsetRaw, data []byte) (map[uint32]int, error) {
	hdr := 4
	cnt := bss.U32(offsetRaw, 0)
	if string(offsetRaw[0:4]) == "PABR" {
		hdr, cnt = 8, bss.U32(offsetRaw, 4)
	}
	if hdr+int(cnt)*12 > len(offsetRaw) {
		return nil, fmt.Errorf("itemmaxlevel offset: bad count %d", cnt)
	}
	out := make(map[uint32]int, cnt)
	for i := 0; i < int(cnt); i++ {
		base := hdr + i*12
		key := bss.U32(offsetRaw, base)   // col0 = item id
		off := bss.U32(offsetRaw, base+4) // col1 = byte offset
		if int(off)+5 <= len(data) {
			out[key] = int(data[off+4])
		}
	}
	return out, nil
}
