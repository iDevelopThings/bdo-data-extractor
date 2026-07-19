package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

const lightstoneSkillLevel = 1

// LightstoneCombinationRow is one combination definition from lightstoneset.bss.
type LightstoneCombinationRow struct {
	Key           uint32
	SkillKey      uint32
	RequiredItems []uint32
	DescriptionKR string
}

// SkillIndexKey returns the level-one key used by skilloffset.dbss.
func (r LightstoneCombinationRow) SkillIndexKey() uint32 {
	return uint32(uint16(r.SkillKey))<<16 | lightstoneSkillLevel
}

// LightstoneItemAlias makes an alternate lightstone count as its canonical item
// when the client tests a combination. Amplified lightstones use this relation.
type LightstoneItemAlias struct {
	Item     uint32
	CountsAs uint32
}

// DecodeLightstoneCombinations reads lightstoneset.bss. Rows contain a fixed
// three-item requirement plus an optional fourth item, followed by a global
// item-equivalence map.
func DecodeLightstoneCombinations(data []byte) ([]LightstoneCombinationRow, []LightstoneItemAlias, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, nil, fmt.Errorf("lightstoneset: %w", err)
	}
	strings := bss.ReadUTF16StringTable(data, pabr.StringTablePos)
	if len(strings) != pabr.Rows {
		return nil, nil, fmt.Errorf("lightstoneset: string table has %d rows, want %d", len(strings), pabr.Rows)
	}

	c := bss.NewCursor(data, pabr.RecordsStart, pabr.StringTablePos)
	rows := make([]LightstoneCombinationRow, 0, pabr.Rows)
	seen := make(map[uint32]bool, pabr.Rows)
	for i := 0; i < pabr.Rows; i++ {
		row := LightstoneCombinationRow{
			Key:      c.U32(),
			SkillKey: c.U32(),
		}
		if !c.Zero(2) {
			return nil, nil, fmt.Errorf("lightstoneset: row %d has a nonzero reserved field", i)
		}
		row.RequiredItems = c.U32N(3)

		// The table omits a count for its 3-or-4-item array. Item ids occupy a
		// separate, much larger key space than the following string-table index.
		if c.PeekU32() >= uint32(len(strings)) {
			row.RequiredItems = append(row.RequiredItems, c.U32())
		}
		descriptionIndex := c.U32()
		if !c.OK() || descriptionIndex >= uint32(len(strings)) {
			return nil, nil, fmt.Errorf("lightstoneset: row %d has invalid description index %d", i, descriptionIndex)
		}
		if row.Key == 0 || row.SkillKey == 0 {
			return nil, nil, fmt.Errorf("lightstoneset: row %d has an empty key", i)
		}
		if seen[row.Key] {
			return nil, nil, fmt.Errorf("lightstoneset: duplicate combination key %d", row.Key)
		}
		seen[row.Key] = true
		for _, itemID := range row.RequiredItems {
			if itemID == 0 {
				return nil, nil, fmt.Errorf("lightstoneset: combination %d has an empty required item", row.Key)
			}
		}
		row.DescriptionKR = strings[descriptionIndex]
		rows = append(rows, row)
	}

	aliasCountRaw := c.U32()
	if uint64(aliasCountRaw)*8 > uint64(c.Remaining()) {
		return nil, nil, fmt.Errorf("lightstoneset: invalid alias count %d", aliasCountRaw)
	}
	aliasCount := int(aliasCountRaw)
	aliases := make([]LightstoneItemAlias, 0, aliasCount)
	for i := 0; i < aliasCount; i++ {
		alias := LightstoneItemAlias{Item: c.U32(), CountsAs: c.U32()}
		if alias.Item == 0 || alias.CountsAs == 0 {
			return nil, nil, fmt.Errorf("lightstoneset: alias %d has an empty item", i)
		}
		aliases = append(aliases, alias)
	}
	if !c.OK() || c.Remaining() != 0 {
		return nil, nil, fmt.Errorf("lightstoneset: records consumed %d of %d bytes", c.Pos(), pabr.StringTablePos)
	}
	return rows, aliases, nil
}
