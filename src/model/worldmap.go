package model

// WorldMap is the region-map base image plus the world->image transform, so any
// world-coordinate dataset (fishing spots, region/node spawns, NPCs) can be placed
// on the map. Written to worldmap.json.
type WorldMap struct {
	Image     string       `json:"image"`
	Width     int          `json:"width"`
	Height    int          `json:"height"`
	Transform WorldToImage `json:"transform"`
}

// WorldToImage is the affine that maps a world (x,z) to image pixel coordinates:
//
//	px = PX[0]*x + PX[1]*z + PX[2]
//	py = PY[0]*x + PY[1]*z + PY[2]
type WorldToImage struct {
	PX [3]float64 `json:"px"`
	PY [3]float64 `json:"py"`
}

// Apply maps a world (x,z) to image pixel coordinates.
func (t WorldToImage) Apply(x, z float64) (px, py float64) {
	return t.PX[0]*x + t.PX[1]*z + t.PX[2], t.PY[0]*x + t.PY[1]*z + t.PY[2]
}
