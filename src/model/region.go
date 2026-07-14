package model

// Spawn is one NPC/monster placement within a region: its character id, world
// position, and dialog/spawn variant index. Placements live on WorldRegion.Spawns.
type Spawn struct {
	Key         uint32     `json:"key"`
	Pos         [3]float64 `json:"pos"`
	DialogIndex int        `json:"dialogIndex,omitempty"`
}

// Bounds is a region's world-space extent (union of its spatial boxes).
type Bounds struct {
	Min [3]float64 `json:"min"`
	Max [3]float64 `json:"max"`
}
