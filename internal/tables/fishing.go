package tables

import (
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// DecodeFishingPoints decodes floatfishingpoint.dbss — a plaintext table:
// [u32 count] then count fixed 42-byte records:
//
//	+0 u16 id   +2 u32 range   +6/+10/+14 f32 x1,y1,z1   +18/+22/+26 f32 x2,y2,z2
//	+30 u16 id(dup)   +32 u32 0   +36 u16 regionGroup   +38 u16 areaType   +40 u16 0
func DecodeFishingPoints(data []byte) []model.FishingPoint {
	const rec = 42
	if len(data) < 4 {
		return nil
	}
	count := int(bss.U32(data, 0))
	if count <= 0 || 4+count*rec > len(data) {
		return nil
	}

	out := make([]model.FishingPoint, 0, count)
	for i := 0; i < count; i++ {
		o := 4 + i*rec
		out = append(out, model.FishingPoint{
			ID:          uint32(bss.U16(data, o)),
			Range:       bss.U32(data, o+2),
			Pos:         []float64{round2(bss.F32(data, o+6)), round2(bss.F32(data, o+10)), round2(bss.F32(data, o+14))},
			Pos2:        []float64{round2(bss.F32(data, o+18)), round2(bss.F32(data, o+22)), round2(bss.F32(data, o+26))},
			RegionGroup: uint32(bss.U16(data, o+36)),
			AreaType:    uint32(bss.U16(data, o+38)),
		})
	}

	return out
}

// WorldToImage is an affine map from world (X,Z) to regionmap pixel (px,py):
//
//	px = PX[0]*X + PX[1]*Z + PX[2]
//	py = PY[0]*X + PY[1]*Z + PY[2]
//
// It lets any world-coordinate dataset (fishing spots, region/node spawns, NPCs)
// be placed on the region map. The transform type + Apply live in src/model.

// FitRegionTransform fits the world<->image affine from regions whose key appears
// both in the region map (image bbox center) and in the spawn data (world centroid),
// and returns the world->image direction. ok is false if too few correspondences.
func FitRegionTransform(rm *RegionMap, worldCenter map[uint32][2]float64) (t model.WorldToImage, matched int, ok bool) {
	if rm == nil {
		return t, 0, false
	}
	var px, py, wx, wz []float64
	for _, rc := range rm.Regions {
		wc, has := worldCenter[rc.Key]
		if rc.Key == 0 || !has {
			continue
		}
		px = append(px, float64(rc.BBox[0])+float64(rc.BBox[2])/2)
		py = append(py, float64(rc.BBox[1])+float64(rc.BBox[3])/2)
		wx = append(wx, wc[0])
		wz = append(wz, wc[1])
	}
	matched = len(px)
	if matched < 8 {
		return t, matched, false
	}

	// fit image->world, then invert the 2x2 linear part to get world->image
	ax, az := fitAffineRobust(px, py, wx, wz)
	a, b, c := ax[0], ax[1], ax[2]
	d, e, f := az[0], az[1], az[2]
	det := a*e - b*d
	if det == 0 {
		return t, matched, false
	}
	t.PX = [3]float64{e / det, -b / det, (b*f - e*c) / det}
	t.PY = [3]float64{-d / det, a / det, (d*c - a*f) / det}
	return t, matched, true
}

// AttributeFishingPoints fills RegionKey/RegionName for each point by mapping its
// world position into regionmap_new's pixel grid (via the fitted transform) and
// reading the region index there. Returns the transform plus correspondence and
// attribution counts.
func AttributeFishingPoints(points []model.FishingPoint, rm *RegionMap, worldCenter map[uint32][2]float64, names map[uint32]string) (t model.WorldToImage, matched, attributed int) {
	if rm == nil || len(rm.Pixels) == 0 {
		return t, 0, 0
	}
	t, matched, ok := FitRegionTransform(rm, worldCenter)
	if !ok {
		return t, matched, 0
	}

	idxKey := make(map[uint16]uint32, len(rm.Regions))
	for _, rc := range rm.Regions {
		idxKey[rc.Index] = rc.Key
	}

	for i := range points {
		fx, fy := t.Apply(points[i].Pos[0], points[i].Pos[2])
		ix, iy := int(fx+0.5), int(fy+0.5)
		if ix < 0 || iy < 0 || ix >= rm.Width || iy >= rm.Height {
			continue
		}
		idx := rm.Pixels[iy*rm.Width+ix]
		if idx == 0 {
			continue
		}
		key := idxKey[idx]
		if key == 0 {
			continue
		}
		points[i].RegionKey = key
		points[i].RegionName = names[key]
		attributed++
	}
	return t, matched, attributed
}

// fitAffineRobust fits world = [a,b,c]·[px,py,1] for X and Z, then drops the worst
// 10% of correspondences by residual and refits — region spawn centroids are noisy,
// so a few far-off regions would otherwise skew the global transform.
func fitAffineRobust(px, py, wx, wz []float64) (ax, az [3]float64) {
	ax = fitLinear3(px, py, wx)
	az = fitLinear3(px, py, wz)

	type res struct {
		i int
		e float64
	}
	rs := make([]res, len(px))
	for i := range px {
		dx := ax[0]*px[i] + ax[1]*py[i] + ax[2] - wx[i]
		dz := az[0]*px[i] + az[1]*py[i] + az[2] - wz[i]
		rs[i] = res{i, dx*dx + dz*dz}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].e < rs[j].e })
	keep := len(rs) * 9 / 10
	if keep < 8 {
		return ax, az
	}
	kx, ky, kwx, kwz := make([]float64, keep), make([]float64, keep), make([]float64, keep), make([]float64, keep)
	for j := 0; j < keep; j++ {
		i := rs[j].i
		kx[j], ky[j], kwx[j], kwz[j] = px[i], py[i], wx[i], wz[i]
	}
	return fitLinear3(kx, ky, kwx), fitLinear3(kx, ky, kwz)
}

// fitLinear3 least-squares fits z = p0*x + p1*y + p2 via 3x3 normal equations.
func fitLinear3(x, y, z []float64) [3]float64 {
	var sxx, sxy, sx, syy, sy, sn, sxz, syz, sz float64
	for i := range x {
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
		sx += x[i]
		syy += y[i] * y[i]
		sy += y[i]
		sn++
		sxz += x[i] * z[i]
		syz += y[i] * z[i]
		sz += z[i]
	}
	return solve3(
		sxx, sxy, sx, sxz,
		sxy, syy, sy, syz,
		sx, sy, sn, sz,
	)
}

// solve3 solves the 3x3 system [[a11,a12,a13],[a21,a22,a23],[a31,a32,a33]]·p = [b1,b2,b3]
// by Cramer's rule (zero vector if singular).
func solve3(a11, a12, a13, b1, a21, a22, a23, b2, a31, a32, a33, b3 float64) [3]float64 {
	det := a11*(a22*a33-a23*a32) - a12*(a21*a33-a23*a31) + a13*(a21*a32-a22*a31)
	if det == 0 {
		return [3]float64{}
	}
	d1 := b1*(a22*a33-a23*a32) - a12*(b2*a33-a23*b3) + a13*(b2*a32-a22*b3)
	d2 := a11*(b2*a33-a23*b3) - b1*(a21*a33-a23*a31) + a13*(a21*b3-b2*a31)
	d3 := a11*(a22*b3-b2*a32) - a12*(a21*b3-b2*a31) + b1*(a21*a32-a22*a31)
	return [3]float64{d1 / det, d2 / det, d3 / det}
}
