package tables

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// DecodeRegionBounds parses region_info.xml's <box region_index="N"
// aabb_min_*/aabb_max_*> elements into the union AABB per region id.
func DecodeRegionBounds(data []byte) map[uint32]*model.Bounds {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	out := map[uint32]*model.Bounds{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "box" {
			continue
		}
		key := attrU32(se, "region_index")
		mn := [3]float64{attrFloat(se, "aabb_min_x"), attrFloat(se, "aabb_min_y"), attrFloat(se, "aabb_min_z")}
		mx := [3]float64{attrFloat(se, "aabb_max_x"), attrFloat(se, "aabb_max_y"), attrFloat(se, "aabb_max_z")}
		b := out[key]
		if b == nil {
			out[key] = &model.Bounds{Min: mn, Max: mx}
			continue
		}
		for i := 0; i < 3; i++ {
			if mn[i] < b.Min[i] {
				b.Min[i] = mn[i]
			}
			if mx[i] > b.Max[i] {
				b.Max[i] = mx[i]
			}
		}
	}
	return out
}

func attrFloat(se xml.StartElement, name string) float64 {
	v, _ := strconv.ParseFloat(attrVal(se, name), 64)
	return v
}

// DecodeRegions parses regionclientdata.xml: a flat stream of
// <RegionInfo Key="N"> elements, each containing <SpawnInfo key="<charId>" dialogIndex="i" position="{x,y,z}"/> children.
func DecodeRegions(data []byte) ([]model.Region, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	var regions []model.Region
	cur := -1
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "RegionInfo":
			regions = append(regions, model.Region{Key: attrU32(se, "Key")})
			cur = len(regions) - 1
		case "SpawnInfo":
			if cur < 0 {
				continue
			}
			regions[cur].Spawns = append(regions[cur].Spawns, model.Spawn{
				Key:         attrU32(se, "key"),
				Pos:         parseVec3(attrVal(se, "position")),
				DialogIndex: attrInt(se, "dialogIndex"),
			})
		}
	}
	return regions, nil
}

func attrVal(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func attrU32(se xml.StartElement, name string) uint32 {
	v, _ := strconv.ParseUint(attrVal(se, name), 10, 32)
	return uint32(v)
}

func attrInt(se xml.StartElement, name string) int {
	v, _ := strconv.Atoi(attrVal(se, name))
	return v
}

// parseVec3 parses "{x,y,z}" into a 3-float array.
func parseVec3(s string) [3]float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	var out [3]float64
	for i, p := range strings.SplitN(s, ",", 3) {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.ParseFloat(strings.TrimSpace(p), 64)
	}
	return out
}
