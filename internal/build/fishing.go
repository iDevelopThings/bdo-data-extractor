package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buildFishing decodes float-fishing point locations (client-side; fish-per-spot
// is server-side), attributes each to a region via the fitted world<->image
// transform, and registers fishingspots.json + worldmap.json. Skips if absent;
// fails if present but corrupt.
func (b *Builder) buildFishing() error {
	fpData, ok, err := b.src.ReadIfExists("floatfishingpoint.dbss")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	points := tables.DecodeFishingPoints(fpData)
	if len(points) == 0 {
		return fmt.Errorf("floatfishingpoint: decoded zero spots")
	}

	if err := b.writeFishingWorldMap(points); err != nil {
		return err
	}

	fp, err := b.addJSON("fishingspots.json", points)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("fishing spots: %d -> %s", len(points), fp))

	return nil
}

// writeFishingWorldMap attributes spots onto regionmap_new when present and
// registers worldmap.json. Missing map assets are skipped; read errors fail.
func (b *Builder) writeFishingWorldMap(points []model.FishingPoint) error {
	bkd, ok, err := b.src.ReadIfExists("regionmap_new.bmp.bkd")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rid, _, _ := b.src.ReadIfExists("regionmap_new.bmp.rid") // optional sidecar; missing is fine
	rm := tables.DecodeRegionMap("regionmap_new", bkd, rid)
	if rm == nil {
		return nil
	}

	worldC := make(map[uint32][2]float64, len(b.regions))
	for _, r := range b.regions {
		if len(r.Spawns) == 0 {
			continue
		}
		var sx, sz float64
		for _, s := range r.Spawns {
			sx += s.Pos[0]
			sz += s.Pos[2]
		}
		n := float64(len(r.Spawns))
		worldC[uint32(r.Key)] = [2]float64{sx / n, sz / n}
	}
	t, m, a := tables.AttributeFishingPoints(points, rm, worldC, b.gs.Topography)
	b.logf(fmt.Sprintf("fishing spots: attributed %d/%d to regions (%d key correspondences)", a, len(points), m))

	wm := model.WorldMap{
		Image:     "regionmaps/regionmap_new.png",
		Width:     rm.Width,
		Height:    rm.Height,
		Transform: t,
	}
	wp, err := b.addJSON("worldmap.json", wm)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("worldmap: base + transform -> %s", wp))

	return nil
}
