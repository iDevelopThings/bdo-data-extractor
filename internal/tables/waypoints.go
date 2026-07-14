package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

const waypointRecordSize = 23

// WorldWaypoint is one navigable world-map node from mapdata_realexplore2.bwp.
type WorldWaypoint struct {
	Key          uint32
	Position     [3]float64
	InternalName string
	Flags        [3]byte
	Links        []uint32
}

// DecodeWorldWaypoints decodes the client-side node positions and connection
// graph from mapdata_realexplore2.bwp. The PABR payload is a fixed row table,
// a counted list of directed key pairs, five zero bytes, and a UTF-16 name
// table with one entry per row.
func DecodeWorldWaypoints(data []byte) (map[uint32]WorldWaypoint, error) {
	h, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("world waypoints: %w", err)
	}

	recordsEnd := h.RecordsStart + h.Rows*waypointRecordSize
	if recordsEnd+9 > h.StringTablePos {
		return nil, fmt.Errorf("world waypoints: %d rows exceed the numeric section", h.Rows)
	}

	waypoints := make(map[uint32]WorldWaypoint, h.Rows)
	keys := make([]uint32, h.Rows)
	c := bss.NewCursor(data, h.RecordsStart, h.StringTablePos)
	for i := range h.Rows {
		key := c.U32()
		index := c.U32()
		position := [3]float64{c.F32(), c.F32(), c.F32()}
		flags := [3]byte{c.Byte(), c.Byte(), c.Byte()}
		if !c.OK() {
			return nil, fmt.Errorf("world waypoints: row %d is truncated", i)
		}
		if key == 0 || index != uint32(i) {
			return nil, fmt.Errorf("world waypoints: invalid row %d (key=%d index=%d)", i, key, index)
		}
		if _, exists := waypoints[key]; exists {
			return nil, fmt.Errorf("world waypoints: duplicate key %d", key)
		}
		keys[i] = key
		waypoints[key] = WorldWaypoint{Key: key, Position: position, Flags: flags}
	}

	edgeCount := int(c.U32())
	if edgeCount < 0 || edgeCount > h.Rows*h.Rows {
		return nil, fmt.Errorf("world waypoints: invalid edge count %d", edgeCount)
	}
	for i := 0; i < edgeCount; i++ {
		from, to := c.U32(), c.U32()
		waypoint, exists := waypoints[from]
		if !exists {
			return nil, fmt.Errorf("world waypoints: edge %d has unknown source %d", i, from)
		}
		if _, exists := waypoints[to]; !exists {
			return nil, fmt.Errorf("world waypoints: edge %d has unknown target %d", i, to)
		}
		waypoint.Links = append(waypoint.Links, to)
		waypoints[from] = waypoint
	}
	if reserved := c.Bytes(5); !c.OK() || !allZero(reserved) || c.Pos() != h.StringTablePos {
		return nil, fmt.Errorf("world waypoints: numeric section ended at %d, want %d", c.Pos(), h.StringTablePos)
	}

	names := bss.ReadUTF16StringTable(data, h.StringTablePos)
	if len(names) != h.Rows {
		return nil, fmt.Errorf("world waypoints: got %d names for %d rows", len(names), h.Rows)
	}
	for i, key := range keys {
		waypoint := waypoints[key]
		waypoint.InternalName = names[i]
		waypoints[key] = waypoint
	}

	return waypoints, nil
}
