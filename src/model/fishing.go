package model

// FishingPoint is one float-fishing spot: its client-side world position (plus a
// second point/extent), a cast range, and the region group / area type it belongs
// to. RegionKey/RegionName are resolved from the region map during the build.
type FishingPoint struct {
	ID          uint32    `json:"id"`
	Range       uint32    `json:"range,omitempty"` // cast range (1000/2000)
	Pos         []float64 `json:"pos"`             // x1,y1,z1
	Pos2        []float64 `json:"pos2,omitempty"`  // x2,y2,z2 (extent / second point; y2 usually 0)
	RegionGroup uint32    `json:"regionGroup,omitempty"`
	AreaType    uint32    `json:"areaType,omitempty"`
	RegionKey   uint32    `json:"regionKey,omitempty"`  // region the spot falls in (via regionmap_new)
	RegionName  string    `json:"regionName,omitempty"` // place/topography name (river/sea/etc.)
}
