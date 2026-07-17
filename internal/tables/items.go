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
	ItemType      string           // EItemType         (byte @4)
	Category      string           // EItemClassify     (byte @5)
	Grade         model.ItemGrade  // EItemGradeType    (byte @6)
	EquipType     string           // EquipType / slot  (byte @7)
	MarketCatID   byte             // central-market main category id (byte @200); named via loc table 44
	MarketSubID   byte             // central-market sub category id  (byte @201)
	SkillKey      uint32           // skill key (u32 @204) -> skill.dbss -> buff effect chain
	Weight        float64          // int32 @63, value/10000 (LT)
	Buy           int64            // OriginalPrice, int64 @110
	Sell          int64            // SellPriceToNpc, int64 @118
	Repair        int64            // RepairPrice, int32 @126
	MaxDurability int              // u16 @197, equipment only (0x7FFF/large = no-durability sentinel)
	Classes       []string         // usable classes (mask @77), nil = all classes / unrestricted
	Slot          model.SlotName   // normalized equip slot (byte @14), equipment only
	Kind          string           // coarse equip kind: Weapon/Armor/Other (byte @15), equipment only
	ExtraSlots    []model.SlotName // additional occupied slots (@16-18), multi-slot costumes only
	Expiration    int              // u32 @69, item expiration in MINUTES (0 = permanent)
	RequiredLevel int              // byte @97, required character level (0/1 = none)
	MaxStack      int64            // u32 @101, max stack size (sentinels = unlimited -> 0)
	DyeParts      int              // byte @172, dyeable part count
	JewelGroup    int              // u16 @ len-4 (record footer), crystal transfusion group; -1 = none
	Icon          string           // "New_Icon/....dds" embedded in the row

	// Fields following the variable icon string:
	Marketable          bool           // iconEnd+0 — may be listed on the central market
	BindType            model.BindType // iconEnd+15 — vestedType (see model.BindType constants)
	MarketRegisterLimit int64          // after three inline UTF-16 strings — max units per market registration

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
	PersonalTrade bool  // @196 IsPersonalTrade (player-to-player handable; 329 items)
	NodeFreeTrade bool  // @202 NodeFreeTrade

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

const (
	itemRowMin = 126 + 4 // smallest row we can fully read

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
		if !ok {
			return nil, fmt.Errorf("itemenchant item %d: invalid indexed slice", e.Key)
		}
		if len(rec) < itemRowMin {
			return nil, fmt.Errorf("itemenchant item %d: record size %d, want at least %d", e.Key, len(rec), itemRowMin)
		}
		if bss.U32(rec, 0) != e.Key {
			return nil, fmt.Errorf("itemenchant item %d: record id %d", e.Key, bss.U32(rec, 0))
		}
		stat, err := decodeItemRow(rec, e.Key)
		if err != nil {
			return nil, err
		}
		out[e.Key] = stat
	}
	ownItemTails(out)
	return out, nil
}

// ownItemTails compacts the preserved unknown tails so returned rows do not
// retain the much larger itemenchant source buffer.
func ownItemTails(stats map[uint32]ItemStat) {
	total := 0
	for _, stat := range stats {
		total += len(stat.U.UnknownPostIconTail)
	}
	if total == 0 {
		return
	}

	arena := make([]byte, total)
	pos := 0
	for id, stat := range stats {
		n := copy(arena[pos:], stat.U.UnknownPostIconTail)
		stat.U.UnknownPostIconTail = arena[pos : pos+n : pos+n]
		stats[id] = stat
		pos += n
	}
}

// decodeItemRow reads one itemenchant row as a straight sequential field stream.
// The fixed header (@0-208) is read in order with a bss.Cursor — no
// offset-jumping. The name and the post-icon block are reached via the icon
// anchor (the icon-string end): the variable enchant block before the name
// isn't reliably sequential — ship-upgrade gear uses a different layout — so the
// anchor keeps those fields correct for every row. The tail beyond the post-icon
// block is item-type-specific: its confirmed prefix is decoded and its remaining
// bytes are preserved in ItemUnknowns. No bytes are silently skipped.
func decodeItemRow(rec []byte, id uint32) (ItemStat, error) {
	var st ItemStat
	c := bss.NewCursor(rec, 0, len(rec))
	reservedOK := true

	// --- fixed header, every byte read in order (no skipping) ---
	// Identified fields land on named struct fields; each unidentified byte is
	// read into its own st.U.Unknown<off> field (deviation-only). Constant runs
	// are consumed and validated so a layout shift fails at the record boundary.
	c.U32()              // @0    id (== key)
	itemType := c.Byte() // @4
	st.ItemType = name(itemTypeNames, itemType)
	st.Category = name(classifyNames, c.Byte())                // @5
	st.Grade = model.ItemGrades.FromWireUnsafe(int8(c.Byte())) // @6
	st.EquipType = name(equipTypeNames, c.Byte())              // @7
	st.U.Unknown8 = dev(c.U8(), 0)                             // @8    flag cluster
	st.U.Unknown9 = dev(c.U8(), 0)                             // @9
	st.U.Unknown10 = dev(c.U8(), 0)                            // @10
	st.U.Unknown11 = dev(c.U8(), 0)                            // @11   set on 26,406 items
	st.U.Unknown12 = dev(c.U8(), 2)                            // @12   default 2
	st.U.Unknown13 = dev(c.U8(), 0)                            // @13
	slot := c.Byte()                                           // @14
	kind := c.Byte()                                           // @15
	extra := c.U8N(3)                                          // @16-18  additional occupied slots
	reservedOK = c.Repeated(43, slotNone)                      // @19-61 fixed 0x2e filler
	st.ItemMaterial = c.U8()                                   // @62
	st.Weight = float64(c.I32()) / 10000.0                     // @63
	st.Stackable = c.Bool()                                    // @67
	st.ApplyDirectly = c.Bool()                                // @68
	st.Expiration = int(c.U32())                               // @69
	st.VestedType = c.U8()                                     // @73
	st.UserVested = c.Bool()                                   // @74
	st.ForTrade = c.Bool()                                     // @75
	st.TradeType = c.U8()                                      // @76
	classMask := c.U64()                                       // @77
	st.U.Unknown85 = dev(int(c.U32()), 0)                      // @85-88  u32 bitfield (item property flags)
	reservedOK = c.Zero(4) && reservedOK                       // @89-92 reserved
	st.U.Unknown93 = dev(c.U8(), 0)                            // @93     (== @98 on all 105 items that set it)
	reservedOK = c.Zero(3) && reservedOK                       // @94-96 reserved
	reqLevel := c.U8()                                         // @97
	st.U.Unknown98 = dev(c.U8(), 0)                            // @98
	st.U.Unknown99 = dev(c.U8(), 0)                            // @99
	st.U.Unknown100 = dev(c.U8(), 0)                           // @100
	maxStack := c.U32()                                        // @101
	st.LifeExpType = c.U8()                                    // @105
	st.U.Unknown106 = dev(c.U8(), 0)                           // @106
	st.U.Unknown107 = dev(c.U8(), 0)                           // @107
	st.U.Unknown108 = dev(c.U8(), 0)                           // @108
	reservedOK = c.Zero(1) && reservedOK                       // @109 reserved
	st.Buy = c.I64()                                           // @110
	st.Sell = c.I64()                                          // @118
	st.Repair = int64(c.I32())                                 // @126
	reservedOK = c.Zero(4) && reservedOK                       // @130-133 reserved
	eventType := c.U8()                                        // @134
	st.U.Unknown135 = dev(c.U8(), 0)                           // @135
	eventP1, eventP2 := c.U32(), c.U32()                       // @136, @140
	st.U.Unknown144 = dev(int(c.U16()), 0)                     // @144-145 (u16)
	st.U.Unknown146 = dev(int(c.U32()), 0)                     // @146-149 u32
	st.U.Unknown150 = dev(int(c.U16()), 0)                     // @150-151 u16
	st.U.Unknown152 = dev(int(c.U16()), 0)                     // @152-153 u16
	st.U.Unknown154 = dev(int(c.U32()), 0)                     // @154-157 u32
	st.U.Unknown158 = dev(c.U8(), 0)                           // @158
	st.U.Unknown159 = dev(c.U8(), 0)                           // @159, 255 sentinel on some rows
	st.U.Unknown160 = dev(c.U8(), 0)                           // @160
	st.U.Unknown161 = dev(c.U8(), 255)                         // @161 default 255
	st.U.Unknown162 = dev(c.U8(), 255)                         // @162 default 255
	hideFromNote := c.U8()                                     // @163
	st.Cash = c.Bool()                                         // @164
	st.CronEnchant = c.U8()                                    // @165
	st.U.Unknown166 = dev(c.U8(), 0)                           // @166
	st.U.Unknown167 = dev(c.U8(), 0)                           // @167
	st.Dyeable = c.Bool()                                      // @168
	st.U.Unknown169 = dev(c.U8(), 1)                           // @169 default 1
	st.U.Unknown170 = dev(int(c.U16()), 0)                     // @170-171 u16
	dyeParts := c.U8()                                         // @172
	st.U.Unknown173 = dev(c.U8(), 0)                           // @173
	st.U.Unknown174 = dev(int(c.U16()), 0)                     // @174-175 u16
	st.U.Unknown176 = dev(int(c.U16()), 0)                     // @176-177 u16
	st.U.Unknown178 = dev(int(c.U16()), 0)                     // @178-179 u16
	st.U.Unknown180 = dev(c.U8(), 0)                           // @180 paired with @188
	st.U.Unknown181 = dev(c.U8(), 0)                           // @181
	st.U.Unknown182 = dev(int(c.U32()), 0)                     // @182-185 u32
	reservedOK = c.Zero(2) && reservedOK                       // @186-187 reserved
	st.U.Unknown188 = dev(c.U8(), 0)                           // @188 paired with @180
	st.U.Unknown189 = dev(c.U8(), 0)                           // @189
	st.U.Unknown190 = dev(c.U8(), 0)                           // @190
	reservedOK = c.Zero(1) && reservedOK                       // @191 reserved
	st.U.Unknown192 = dev(int(c.U16()), 0)                     // @192-193 u16
	reservedOK = c.Zero(2) && reservedOK                       // @194-195 reserved
	st.PersonalTrade = c.Bool()                                // @196
	maxDur := c.U16()                                          // @197
	st.U.Unknown199 = dev(c.U8(), 0)                           // @199
	st.MarketCatID = c.Byte()                                  // @200
	st.MarketSubID = c.Byte()                                  // @201
	st.NodeFreeTrade = c.Bool()                                // @202
	st.U.Unknown203 = dev(c.U8(), 1)                           // @203 default 1
	st.SkillKey = c.U32()                                      // @204
	if !c.OK() || c.Pos() != 208 {
		return st, fmt.Errorf("itemenchant item %d: header ended at %d, want 208", id, c.Pos())
	}
	if !reservedOK {
		return st, fmt.Errorf("itemenchant item %d: reserved header bytes changed", id)
	}

	// header-derived
	st.ShownInNote = hideFromNote == 0
	if eventType != 0 && eventType != 171 { // 171 = the "no event" default
		st.EventType, st.EventParam1, st.EventParam2 = eventType, int64(eventP1), int64(eventP2)
	}
	if reqLevel > 1 && reqLevel <= 100 {
		st.RequiredLevel = reqLevel
	}
	if maxStack > 0 && maxStack != 0x7FFFFFFF && maxStack != 0xFFFFFF00 { // sentinels = unlimited
		st.MaxStack = int64(maxStack)
	}
	if dyeParts > 0 && dyeParts <= 30 {
		st.DyeParts = dyeParts
	}

	if itemType == eItemTypeEquip {

		st.Classes = ClassRestriction(classMask)
		st.Slot = model.SlotName(slot)
		st.Kind = name(equipKindNames, kind)
		for i := 0; i < 3; i++ { // extra occupied slots @16-18
			if extra[i] != slotNone {
				st.ExtraSlots = append(st.ExtraSlots, model.SlotName(extra[i]))
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
		if icEnd >= ic && icEnd+18 <= len(rec)-8 {
			p := bss.NewCursor(rec, icEnd, len(rec)-8)
			postReservedOK := true
			st.Marketable = p.Bool()                     // +0
			st.U.UnknownIcon1 = dev(p.U8(), 0)           // +1
			st.U.UnknownIcon2 = dev(p.U8(), 9)           // +2   default 9 (currencies deviate)
			st.U.UnknownIcon3 = dev(p.U8(), 0)           // +3
			postReservedOK = p.Zero(4)                   // +4..+7 reserved
			st.U.UnknownIcon8 = dev(p.U8(), 2)           // +8    Cook-product flag (best guess)
			st.U.UnknownIcon9 = dev(p.U8(), 0)           // +9
			st.U.UnknownIcon10 = dev(p.U8(), 0)          // +10   food tier (best guess)
			postReservedOK = p.Zero(2) && postReservedOK // +11..+12 reserved
			familyInventory := p.U8()                    // +13   0 or 2
			st.FamilyInventory = familyInventory == 2
			st.U.UnknownIcon14 = dev(p.U8(), 0)          // +14
			st.BindType = model.BindType(p.U8())         // +15   vestedType
			st.U.UnknownIcon16 = dev(p.U8(), 1)          // +16   resource class (best guess)
			postReservedOK = p.Zero(1) && postReservedOK // +17 reserved
			for i := range st.U.UnknownPostIconStrings {
				st.U.UnknownPostIconStrings[i] = p.UTF16()
			}
			if lim := p.I64(); lim > 0 && lim < 1<<32 {
				st.MarketRegisterLimit = lim
			}

			// Fixed prefix following the variable strings and market limit.
			st.U.UnknownAfterMarketLimit0 = dev(p.U8(), 0)
			st.U.UnknownAfterMarketLimit1 = dev(int(p.U32()), 0)
			st.U.UnknownAfterMarketLimit5 = dev(int(p.U32()), 0)
			for i := range st.U.UnknownAfterMarketLimitSlots {
				st.U.UnknownAfterMarketLimitSlots[i] = p.Byte()
			}
			postReservedOK = p.Repeated(43, 0x77) && postReservedOK
			st.U.UnknownAfterMarketLimit55 = dev(p.U8(), 0)
			st.U.UnknownAfterMarketLimit56 = dev(p.U8(), 0)
			postReservedOK = p.Zero(1) && postReservedOK
			st.U.UnknownAfterMarketLimit58 = dev(p.U8(), 0)
			st.U.UnknownAfterMarketLimit59 = dev(p.U8(), 0)
			st.U.UnknownAfterMarketLimit60 = dev(int(p.U32()), 1_000_000)
			st.U.UnknownAfterMarketLimit64 = dev(int(p.U32()), 1_000_000)
			st.U.UnknownAfterMarketLimit68 = dev(int(p.U32()), 1_000_000)
			st.U.UnknownAfterMarketLimit72 = dev(int(p.U32()), 1_000_000)
			st.U.UnknownAfterMarketLimit76 = dev(int(p.U32()), 1_000_000)
			st.U.UnknownAfterMarketLimit80 = dev(int(p.U32()), 1_000_000)
			st.U.UnknownPostIconTail = p.Bytes(p.Remaining())
			if !p.OK() || !postReservedOK {
				return st, fmt.Errorf("itemenchant item %d: post-icon layout changed at %d", id, p.Pos()-icEnd)
			}
			st.ContributionCost = contributionCostOf(rec, icEnd)
		} else {
			return st, fmt.Errorf("itemenchant item %d: invalid icon string boundary", id)
		}
	} else {
		// Two live rows use legacy icon paths. Preserve their variable section.
		if len(rec) < 216 {
			return st, fmt.Errorf("itemenchant item %d: record too short for post-icon tail (%d bytes)", id, len(rec))
		}
		st.U.UnknownPostIconTail = rec[208 : len(rec)-8]
	}

	// --- footer: crystal transfusion group (self-id echo then u16 group) ---
	st.JewelGroup = -1
	if l := len(rec); l < 12 || bss.U32(rec, l-8) != id {
		return st, fmt.Errorf("itemenchant item %d: invalid footer", id)
	}
	if g := bss.U16(rec, len(rec)-4); g != noJewelGroup {
		st.JewelGroup = int(g)
	}
	st.U.UnknownFooter6 = dev(int(bss.U16(rec, len(rec)-2)), 0)
	return st, nil
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
