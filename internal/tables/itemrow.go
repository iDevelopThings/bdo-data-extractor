package tables

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// RowSpan is one contiguous region of an itemenchant row. TileItemRow tiles the
// ENTIRE record into ordered spans — every known field renders its decoded
// value, and every byte we haven't identified lands in an explicit "?" span
// with its hex, so unknown regions stay visible instead of silently skipped
// (that visibility is how several fields here were found).
type RowSpan struct {
	Off   int
	Len   int
	Name  string // "?" spans are unidentified
	Value string
}

// itemRowField is a known fixed-header field (offset from record start).
type itemRowField struct {
	off, size int
	name      string
	dec       func(rec []byte, off int) string
}

func decU8(rec []byte, o int) string  { return fmt.Sprintf("%d", rec[o]) }
func decU16(rec []byte, o int) string { return fmt.Sprintf("%d", bss.U16(rec, o)) }
func decU32(rec []byte, o int) string { return fmt.Sprintf("%d", bss.U32(rec, o)) }
func decI32(rec []byte, o int) string { return fmt.Sprintf("%d", int32(bss.U32(rec, o))) }
func decI64(rec []byte, o int) string { return fmt.Sprintf("%d", int64(bss.U64(rec, o))) }
func decU64x(rec []byte, o int) string {
	return fmt.Sprintf("0x%016x", bss.U64(rec, o))
}

// the validated fixed-header fields of an item row, in offset order (§3 of
// FORMATS.md). Gaps between them become "?" spans.
var itemRowHeader = []itemRowField{
	{0, 4, "itemId", decU32},
	{offItemType, 1, "itemType", func(r []byte, o int) string { return fmt.Sprintf("%d (%s)", r[o], name(itemTypeNames, r[o])) }},
	{offClassify, 1, "classify", func(r []byte, o int) string { return fmt.Sprintf("%d (%s)", r[o], name(classifyNames, r[o])) }},
	{offGrade, 1, "grade", func(r []byte, o int) string { return fmt.Sprintf("%d (%s)", r[o], name(gradeNames, r[o])) }},
	{offEquipType, 1, "equipType", func(r []byte, o int) string { return fmt.Sprintf("%d (%s)", r[o], name(equipTypeNames, r[o])) }},
	{offSlot, 1, "equipSlot", decU8},
	{offKind, 1, "equipKind", decU8},
	{offExtraSlot, 3, "extraSlots", func(r []byte, o int) string { return fmt.Sprintf("%d,%d,%d", r[o], r[o+1], r[o+2]) }},
	{62, 1, "itemMaterial", decU8},
	{offWeight, 4, "weight(x10000)", decI32},
	{67, 1, "isStack", decU8},
	{68, 1, "applyDirectly", decU8},
	{offExpiration, 4, "expirationMin", decU32},
	{73, 1, "vestedType", decU8},
	{74, 1, "userVested", decU8},
	{75, 1, "forTrade", decU8},
	{76, 1, "tradeType", decU8},
	{offClassMask, 8, "classMask", decU64x},
	{offReqLevel, 1, "requiredLevel", decU8},
	{offMaxStack, 4, "maxStack", decU32},
	{105, 1, "lifeExpType", decU8},
	{offBuy, 8, "buyPrice", decI64},
	{offSell, 8, "sellPrice", decI64},
	{offRepair, 4, "repairPrice", decI32},
	{134, 1, "eventType", decU8},
	{136, 4, "eventParam1", decU32},
	{140, 4, "eventParam2", decU32},
	{151, 1, "hideFromNote", decU8},
	{152, 1, "isCash", decU8},
	{153, 1, "cronEnchantControl", decU8},
	{156, 1, "isDyeable", decU8},
	{offDyeParts, 1, "dyeParts", decU8},
	{184, 1, "personalTrade", decU8},
	{offDurability, 2, "maxDurability", decU16},
	{offMarketCat, 1, "marketCat", decU8},
	{offMarketCat + 1, 1, "marketSub", decU8},
	{190, 1, "nodeFreeTrade", decU8},
	{192, 4, "skillKey", func(r []byte, o int) string { return fmt.Sprintf("0x%08x", bss.U32(r, o)) }},
}

// known subfields of the post-icon fixed block (iconEnd-relative).
var itemRowIconBlock = []itemRowField{
	{0, 1, "marketable", decU8},
	{15, 1, "bindType", decU8},
	{42, 4, "marketRegisterLimit", decI32},
}

func hexSpan(rec []byte, off, end int) string {
	b := rec[off:end]
	if len(b) > 64 {
		return fmt.Sprintf("% x … (+%d more)", b[:64], len(b)-64)
	}
	return fmt.Sprintf("% x", b)
}

// TileItemRow tiles one itemenchant item row into ordered spans covering every
// byte: fixed header fields (with "?" gaps), the variable pre-name region, the
// Name/EnchantKey/Icon group, the post-icon block, the embedded description,
// and the footer.
func TileItemRow(rec []byte) []RowSpan {
	var spans []RowSpan
	pos := 0
	emitGap := func(to int, label string) {
		if to > pos {
			spans = append(spans, RowSpan{pos, to - pos, label, hexSpan(rec, pos, to)})
			pos = to
		}
	}
	emit := func(off, size int, name, val string) {
		emitGap(off, "?")
		spans = append(spans, RowSpan{off, size, name, val})
		pos = off + size
	}

	// fixed header
	for _, f := range itemRowHeader {
		if f.off+f.size > len(rec) {
			break
		}
		emit(f.off, f.size, f.name, f.dec(rec, f.off))
	}

	// name / enchantKey / icon (positional, icon-anchored)
	ic := bytes.Index(rec, iconMarker)
	if ic >= 16 {
		iconLenOff := ic - 8
		iconLen := int(int64(bss.U64(rec, iconLenOff)))
		// locate the name via its length prefix ending at the icon prefix
		nameOff, nameLen := -1, 0
		for nl := 1; nl <= 80; nl++ {
			no := iconLenOff - 8 - nl*2
			if no < 4 {
				break
			}
			if int(int64(bss.U64(rec, no))) == nl {
				nameOff, nameLen = no, nl
				break
			}
		}
		if nameOff >= 4 {
			emit(nameOff-4, 4, "enchantKey", decU32(rec, nameOff-4))
			emit(nameOff, 8, "nameLen", fmt.Sprintf("%d", nameLen))
			emit(nameOff+8, nameLen*2, "name", bss.DecodeUTF16(rec[nameOff+8:nameOff+8+nameLen*2]))
		}
		emit(iconLenOff, 8, "iconLen", fmt.Sprintf("%d", iconLen))
		if ic+iconLen <= len(rec) {
			emit(ic-8+8, iconLen, "icon", string(rec[ic:ic+iconLen]))
			icEnd := ic + iconLen
			// post-icon fixed block
			for _, f := range itemRowIconBlock {
				if icEnd+f.off+f.size > len(rec) {
					break
				}
				emit(icEnd+f.off, f.size, "icon+"+fmt.Sprint(f.off)+" "+f.name, f.dec(rec, icEnd+f.off))
			}
		}
	}

	// embedded Korean description (i64 char count + UTF-16), found by scanning
	// for a plausible length prefix followed by text
	for o := pos; o+8 < len(rec)-16; o++ {
		n := int(int64(bss.U64(rec, o)))
		if n < 8 || n > 8000 || o+8+n*2 > len(rec) {
			continue
		}
		txt := rec[o+8 : o+8+min(24, n*2)]
		score := 0
		for k := 0; k+1 < len(txt); k += 2 {
			u := bss.U16(txt, k)
			if (u >= 0xAC00 && u <= 0xD7A3) || (u >= 32 && u < 127) || u == '\n' {
				score++
			}
		}
		if score >= 10 {
			emitGap(o, "?")
			emit(o, 8, "descLen", fmt.Sprintf("%d", n))
			desc := bss.DecodeUTF16(rec[o+8 : o+8+n*2])
			if len(desc) > 120 {
				desc = desc[:120] + "…"
			}
			emit(o+8, n*2, "desc", strings.ReplaceAll(desc, "\n", "\\n"))
			break
		}
	}

	// footer
	if len(rec) >= 8 {
		emitGap(len(rec)-8, "?")
		emit(len(rec)-8, 4, "footerSelfId", decU32(rec, len(rec)-8))
		emit(len(rec)-4, 2, "crystalGroup", decU16(rec, len(rec)-4))
		emit(len(rec)-2, 2, "footerConst", fmt.Sprintf("0x%04x", bss.U16(rec, len(rec)-2)))
	}
	emitGap(len(rec), "?")
	return spans
}
