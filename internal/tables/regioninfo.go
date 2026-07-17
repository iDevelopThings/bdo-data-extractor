package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// regioninfo.bss is the geographic region database. It is a variable-record
// PABR table with a 210-byte head, two counted lists and a 171-byte tail.
// DecodeRegionInfo preserves every non-reserved field and validates every byte
// classified as reserved. FORMATS.md documents the complete byte layout.
const (
	regionWarehouseLimit     = 512
	regionExtraPositionLimit = 16
	regionTailSize           = 171
)

// DecodeRegionInfo reads regioninfo.bss into the geographic region list, plus
// each territory's capital region key (stored redundantly on every record; the
// decoder validates the redundancy and reports it once per territory). Korean
// names come from the table's own string table; English names are joined by the
// caller (loc table 17 via regionKey, loc table 12 via territoryIndex).
func DecodeRegionInfo(data []byte) ([]model.WorldRegion, map[int]int, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, nil, fmt.Errorf("regioninfo: %w", err)
	}
	strs := bss.ReadUTF16StringTable(data, pabr.StringTablePos)
	if len(strs) == 0 {
		return nil, nil, fmt.Errorf("regioninfo: empty string table")
	}

	out := make([]model.WorldRegion, 0, pabr.Rows)
	capitals := map[int]int{}
	c := bss.NewCursor(data, pabr.RecordsStart, pabr.StringTablePos)
	for i := 0; i < pabr.Rows; i++ {
		recordStart := c.Pos()
		key := c.U16()
		mapColor := [3]uint8{uint8(c.U8()), uint8(c.U8()), uint8(c.U8())}
		reservedOK := c.Zero(1) // +5
		regionType := c.U8()
		villageSiegeDay := c.U8()
		reservedOK = c.Zero(3) && reservedOK // +8..+10
		unknowns := model.WorldRegionUnknowns{
			Unknown11: uint8(c.U8()),
			Unknown12: c.Bool(),
			Unknown13: c.Bool(),
		}
		ocean := c.Bool()
		desert := c.Bool()
		prison := c.Bool()
		sea := c.Bool()
		unknowns.Unknown18 = c.Bool()
		unknowns.Unknown19 = c.Bool()
		unknowns.Unknown20 = c.Bool()
		unknowns.Unknown21 = c.Bool()
		unknowns.Unknown22 = c.Bool()
		unknowns.Unknown23 = c.Bool()
		unknowns.Unknown24 = c.Bool()
		unknowns.Unknown25 = c.Bool()
		unknowns.Unknown26 = c.Bool()
		locator := c.Bool()
		unknowns.Unknown28 = c.Bool()
		unknowns.Unknown29 = uint16(c.U16())
		unknowns.Unknown31 = c.Bool()
		unknowns.Unknown32 = c.U32()
		reservedOK = c.Zero(1) && reservedOK // +36
		unknowns.Unknown37 = c.Bool()
		villainRespawnKey := c.U32()
		villainRespawnPosition := [3]float64{c.F32(), c.F32(), c.F32()}
		unknowns.Unknown54 = c.Bool()
		unknowns.Unknown55 = c.Bool()
		unknowns.Unknown56 = c.Bool()
		unknowns.Unknown57 = c.Bool()
		unknowns.Unknown58 = c.Bool()
		reservedOK = c.Zero(1) && reservedOK // +59
		unknowns.Unknown60 = c.U32()
		reservedOK = c.Zero(2) && reservedOK // +64..+65
		unknowns.Unknown66 = c.Bool()
		reservedOK = c.Zero(1) && reservedOK // +67
		unknowns.Unknown68 = c.U32()
		reservedOK = c.Zero(10) && reservedOK // +72..+81
		unknowns.Unknown82 = c.Bool()
		reservedOK = c.Zero(1) && reservedOK // +83
		unknowns.Unknown84 = c.U32()
		reservedOK = c.Zero(2) && reservedOK // +88..+89
		territory := c.U8()
		reservedOK = c.Zero(1) && reservedOK // +91
		nameIdx := c.U32()
		capitalNameIdx := c.U32()
		capitalKey := c.U16()
		affiliatedTownKey := c.U16()
		regionGroupKey := c.U16()
		reservedOK = c.Zero(1) && reservedOK // +106
		unknowns.Unknown107 = uint16(c.U16())
		reservedOK = c.Zero(2) && reservedOK // +109..+110
		explorationKey := c.U16()
		reservedOK = c.Zero(2) && reservedOK // +113..+114
		unknowns.Unknown115 = c.Bool()
		reservedOK = c.Zero(3) && reservedOK // +116..+118
		waypointPosition := [3]float64{c.F32(), c.F32(), c.F32()}
		position := [3]float64{c.F32(), c.F32(), c.F32()}
		reservedOK = c.Zero(4) && reservedOK // +143..+146
		unknowns.Unknown147 = c.Bool()
		reservedOK = c.Zero(1) && reservedOK // +148
		unknowns.Unknown149 = c.U32()
		for k := range unknowns.Unknown153 {
			unknowns.Unknown153[k] = c.F32()
		}
		unknowns.Unknown173 = c.U32()
		unknowns.Unknown177 = c.U32()
		unknowns.Unknown181 = c.U32()
		for k := range unknowns.Unknown185 {
			unknowns.Unknown185[k] = c.U32()
		}
		unknowns.Unknown209 = c.Bool()

		warehouseCount := int(c.U32())
		if warehouseCount < 0 || warehouseCount > regionWarehouseLimit || c.Remaining() < warehouseCount*2+4+regionTailSize {
			return nil, nil, fmt.Errorf("regioninfo: record %d bad warehouse-group count %d at %d", i, warehouseCount, recordStart)
		}
		warehouseGroup := make([]int, warehouseCount)
		for k := range warehouseGroup {
			warehouseGroup[k] = int(c.U16())
		}

		positionCount := int(c.U32())
		if positionCount < 0 || positionCount > regionExtraPositionLimit || c.Remaining() < positionCount*12+regionTailSize {
			return nil, nil, fmt.Errorf("regioninfo: record %d bad extra-position count %d at %d", i, positionCount, recordStart)
		}
		extraPositions := make([][3]float64, positionCount)
		for k := range extraPositions {
			extraPositions[k] = [3]float64{c.F32(), c.F32(), c.F32()}
		}

		reservedOK = c.Zero(1) && reservedOK // tail +0
		unknowns.UnknownTail1 = uint16(c.U16())
		unknowns.UnknownTail3 = uint16(c.U16())
		for k := range unknowns.UnknownTail5 {
			unknowns.UnknownTail5[k] = c.F32()
		}
		unknowns.UnknownTail17 = c.U32()
		unknowns.UnknownTail21 = uint8(c.U8())
		unknowns.UnknownTail22 = uint8(c.U8())
		unknowns.UnknownTail23 = uint8(c.U8())
		unknowns.UnknownTail24 = c.Bool()
		for k := range unknowns.UnknownTail25 {
			unknowns.UnknownTail25[k] = c.F32()
		}
		unknowns.UnknownTail49 = c.U64()
		unknowns.UnknownTail57 = c.F32()
		unknowns.UnknownTail61 = c.F32()
		unknowns.UnknownTail65 = c.U32()
		unknowns.UnknownTail69 = c.Bool()
		unknowns.UnknownTail70 = c.Bool()
		unknowns.UnknownTail71 = c.U32()
		unknowns.UnknownTail75 = uint16(c.U16())
		unknowns.UnknownTail77 = uint16(c.U16())
		unknowns.UnknownTail79 = uint8(c.U8())
		reservedOK = c.Zero(1) && reservedOK // tail +80
		unknowns.UnknownTail81 = c.F32()
		for k := range unknowns.UnknownTail85 {
			unknowns.UnknownTail85[k] = c.U32()
		}
		unknowns.UnknownTail109 = c.Bool()
		unknowns.UnknownTail110 = [3]float64{c.F32(), c.F32(), c.F32()}
		unknowns.UnknownTail122 = [3]float64{c.F32(), c.F32(), c.F32()}
		unknowns.UnknownTail134 = uint8(c.U8())
		unknowns.UnknownTail135 = uint8(c.U8())
		unknowns.UnknownTail136 = c.Bool()
		unknowns.UnknownTail137 = uint8(c.U8())
		reservedOK = c.Zero(7) && reservedOK // tail +138..+144
		unknowns.UnknownTail145 = uint8(c.U8())
		reservedOK = c.Zero(8) && reservedOK // tail +146..+153
		unknowns.UnknownTail154 = c.U32()
		unknowns.UnknownTail158 = c.U32()
		unknowns.UnknownTail162 = c.U32()
		reservedOK = c.Zero(3) && reservedOK // tail +166..+168
		guildWharfManagerKey := c.U16()

		if !c.OK() {
			return nil, nil, fmt.Errorf("regioninfo: record %d truncated at %d", i, recordStart)
		}
		if !reservedOK {
			return nil, nil, fmt.Errorf("regioninfo: record %d has nonzero reserved data at %d", i, recordStart)
		}
		if int(nameIdx) >= len(strs) {
			return nil, nil, fmt.Errorf("regioninfo: record %d name index %d out of range", i, nameIdx)
		}
		if int(capitalNameIdx) >= len(strs) {
			return nil, nil, fmt.Errorf("regioninfo: record %d capital name index %d out of range", i, capitalNameIdx)
		}
		if previous, seen := capitals[territory]; seen && previous != int(capitalKey) {
			return nil, nil, fmt.Errorf("regioninfo: territory %d has conflicting capitals %d/%d", territory, previous, capitalKey)
		}
		capitals[territory] = int(capitalKey)

		out = append(out, model.WorldRegion{
			BaseFor:                models.NewBaseFor[model.WorldRegion](uint32(key), "region"),
			WorldRegionUnknowns:    unknowns,
			Key:                    int(key),
			Type:                   regionType,
			MapColor:               mapColor,
			VillageSiegeDay:        villageSiegeDay,
			Ocean:                  ocean,
			Desert:                 desert,
			Prison:                 prison,
			Sea:                    sea,
			Locator:                locator,
			Territory:              model.TerritoryRef(territory),
			AffiliatedTown:         model.WorldRegionRef(affiliatedTownKey),
			RegionGroupKey:         int(regionGroupKey),
			Exploration:            model.WorldNodeRef(explorationKey),
			VillainRespawn:         model.WorldNodeRef(villainRespawnKey),
			VillainRespawnPosition: villainRespawnPosition,
			WaypointPosition:       waypointPosition,
			// embedded Korean name; the build's loc join replaces it
			Name:              strs[nameIdx],
			Position:          position,
			ExtraPositions:    extraPositions,
			WarehouseGroup:    warehouseGroup,
			GuildWharfManager: model.NPCRef(guildWharfManagerKey),
		})
	}
	if !c.OK() || c.Remaining() != 0 {
		return nil, nil, fmt.Errorf("regioninfo: %d records leave %d record bytes", pabr.Rows, c.Remaining())
	}
	return out, capitals, nil
}
