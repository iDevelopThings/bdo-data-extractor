package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buildFishing decodes float-fishing point locations (client-side; fish-per-spot
// is server-side), attributes each to a region via the fitted world<->image
// transform, and writes fishingspots.json + worldmap.json. Skips if absent.
func (b *Builder) buildFishing() error {
	fpData, err := b.src.Read("floatfishingpoint.dbss")
	if err != nil {
		return nil
	}
	points := tables.DecodeFishingPoints(fpData)
	if len(points) == 0 {
		return nil
	}

	if bkd, err := b.src.Read("regionmap_new.bmp.bkd"); err == nil {
		rid, _ := b.src.Read("regionmap_new.bmp.rid")
		if rm := tables.DecodeRegionMap("regionmap_new", bkd, rid); rm != nil {
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
				worldC[r.Key] = [2]float64{sx / n, sz / n}
			}
			t, m, a := tables.AttributeFishingPoints(points, rm, worldC, b.gs.Topography)
			b.logf(fmt.Sprintf("fishing spots: attributed %d/%d to regions (%d key correspondences)", a, len(points), m))

			// worldmap.json: the region-map base image + world->image transform,
			// so the viewer can place any world-coordinate data on the map.
			wm := model.WorldMap{
				Image:     "regionmaps/regionmap_new.png",
				Width:     rm.Width,
				Height:    rm.Height,
				Transform: t,
			}
			wp, err := b.write("worldmap.json", wm)
			if err != nil {
				return err
			}
			b.logf(fmt.Sprintf("worldmap: base + transform -> %s", wp))
		}
	}
	fp, err := b.write("fishingspots.json", points)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("fishing spots: %d -> %s", len(points), fp))

	return nil
}
