package pipeline

import (
	"fmt"
	"image"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/internal/tex"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// base is embedded by every source to supply the no-op defaults (no rebuild, no
// redirects, captured source), so each concrete source only writes what differs.
type base struct {
	src *paz.Source
}

func (b *base) Rebuild() bool                                { return false }
func (b *base) Redirects(matched []string) map[string]string { return nil }

// --- item icons -------------------------------------------------------------

// item decodes item icons (DXT-compressed .dds) to shared PNGs under icons/. Items
// share icons heavily, so each unique icon path is decoded once (named by its slug);
// asset_redirects.json then maps every item's id-based request to the shared file.
type item struct {
	base
	archiveOf map[string]string // wanted archive path -> output slug (icons/-relative)
	redirects map[string]string // "icons/<id>.png" -> "icons/<slug>.png"
}

func itemIcons() imageSource {
	return &item{}
}

func (s *item) Name() string  { return "item icons" }
func (s *item) Dir() string   { return "icons" }
func (s *item) Rebuild() bool { return true }

func (s *item) Prepare(src *paz.Source, dataDir string) error {
	s.src = src
	var items []struct {
		ID   uint32 `json:"id"`
		Icon string `json:"icon"`
	}
	if err := jsonio.ReadFile(filepath.Join(dataDir, "items.json"), &items); err != nil {
		return fmt.Errorf("load items.json (run build first): %w", err)
	}
	s.archiveOf = map[string]string{}
	s.redirects = map[string]string{}
	for _, it := range items {
		if it.Icon == "" {
			continue
		}
		low := strings.ReplaceAll(strings.ToLower(it.Icon), "\\", "/")
		slug := utils.IconFileName(low)
		s.archiveOf["ui_texture/icon/"+low] = slug
		s.redirects[fmt.Sprintf("icons/%d%s", it.ID, utils.IconExt)] = "icons/" + slug
	}
	return nil
}

func (s *item) Wants(path string) bool {
	_, ok := s.archiveOf[strings.ToLower(path)]
	return ok
}

func (s *item) Convert(path string, f paz.PazFile) ([]output, error) {
	data := encodeIcon(s.src.Archive, f)
	if data == nil {
		return nil, nil
	}
	return []output{{Rel: s.archiveOf[strings.ToLower(path)], Data: data}}, nil
}

func (s *item) Redirects(matched []string) map[string]string {
	return s.redirects
}

// --- knowledge icons --------------------------------------------------------

// knowledge decodes each knowledge card's encyclopedia image to knowledge_icons/,
// mirroring the image path stored in knowledge.json. Cards mostly share category
// art, so each unique image decodes once; asset_redirects.json maps each card's
// key-based request to the shared file.
type knowledge struct {
	base
	dest      map[string]string // wanted archive path -> output rel path (mirrors source subdir)
	redirects map[string]string // "knowledge_icons/<key>.png" -> shared image
}

func knowledgeIcons() imageSource {
	return &knowledge{}
}

func (s *knowledge) Name() string { return "knowledge icons" }
func (s *knowledge) Dir() string  { return "knowledge_icons" }

func (s *knowledge) Prepare(src *paz.Source, dataDir string) error {
	s.src = src
	var k struct {
		Entries []struct {
			Key   uint32 `json:"key"`
			Image string `json:"image"`
		} `json:"entries"`
	}
	if err := jsonio.ReadFile(filepath.Join(dataDir, "knowledge.json"), &k); err != nil {
		return fmt.Errorf("load knowledge.json (run build first): %w", err)
	}
	s.dest = map[string]string{}
	s.redirects = map[string]string{}
	for _, e := range k.Entries {
		if e.Image == "" {
			continue
		}
		s.dest["ui_texture/"+strings.TrimSuffix(e.Image, utils.IconExt)+".dds"] = e.Image
		s.redirects[fmt.Sprintf("knowledge_icons/%d%s", e.Key, utils.IconExt)] = "knowledge_icons/" + e.Image
	}
	return nil
}

func (s *knowledge) Wants(path string) bool {
	_, ok := s.dest[strings.ToLower(path)]
	return ok
}

func (s *knowledge) Convert(path string, f paz.PazFile) ([]output, error) {
	data := encodeIcon(s.src.Archive, f)
	if data == nil {
		return nil, nil
	}
	return []output{{Rel: s.dest[strings.ToLower(path)], Data: data}}, nil
}

func (s *knowledge) Redirects(matched []string) map[string]string {
	return s.redirects
}

// --- territory icons --------------------------------------------------------

// territory decodes each territory's worldmap-mark textures to icons/territories/,
// named as world.json's iconLarge/iconSmall paths expect. The .dds paths come from
// territoryinfo.bss.
type territory struct {
	base
	want map[string]string // wanted archive path -> output file name
}

func territoryIcons() imageSource {
	return &territory{}
}

func (s *territory) Name() string { return "territory icons" }
func (s *territory) Dir() string  { return "icons/territories" }

func (s *territory) Prepare(src *paz.Source, dataDir string) error {
	s.src = src
	raw, err := src.Read("territoryinfo.bss")
	if err != nil {
		return err
	}
	terrs, err := tables.DecodeTerritories(raw)
	if err != nil {
		return err
	}
	var raws []string
	for _, t := range terrs {
		raws = append(raws, t.IconLarge, t.IconSmall)
	}
	names := tables.TerritoryIconFiles(raws)
	s.want = map[string]string{}
	for _, t := range terrs {
		for _, icon := range []string{t.IconLarge, t.IconSmall} {
			if icon != "" {
				s.want[tables.TerritoryIconArchivePath(icon)] = names[icon]
			}
		}
	}
	return nil
}

func (s *territory) Wants(path string) bool {
	_, ok := s.want[strings.ToLower(path)]
	return ok
}

func (s *territory) Convert(path string, f paz.PazFile) ([]output, error) {
	data := encodeIcon(s.src.Archive, f)
	if data == nil {
		return nil, nil
	}
	return []output{{Rel: s.want[strings.ToLower(path)], Data: data}}, nil
}

// --- zone-category icons ----------------------------------------------------

// zoneCategoryAtlas is the UI sprite sheet holding the Monster Zone Info tab and
// filter-button icons (the icon ids stored in zones.json).
const zoneCategoryAtlas = "ui_texture/combine/etc/combine_etc_dropitem.dds"

// atlasGrid describes a family of uniformly-packed sprites in the atlas: slot 1's
// top-left, the cell size, the horizontal stride between slots, and the vertical
// stride between states (Normal/Over/Click). The per-slot UVs are computed by the
// UI script at runtime (only one template slot is in the XML), so the grid is
// measured from the atlas and verified to reproduce the template UVs exactly.
type atlasGrid struct {
	x0, y0, w, h, hStride, vStride int
}

var zoneIconGrids = map[string]atlasGrid{
	"tab":        {x0: 1, y0: 1, w: 45, h: 45, hStride: 46, vStride: 46},     // region tabs
	"filter_btn": {x0: 240, y0: 314, w: 42, h: 42, hStride: 43, vStride: 43}, // content filters
}

var iconStates = []struct {
	suffix string
	idx    int
}{{"_normal", 0}, {"_over", 1}, {"_click", 2}}

// zoneIconRect maps an icon id like "Combine_Etc_DropItem_Icon_Tab_05" or
// "..._Filter_Btn_05_Over" to its rectangle in the atlas. Main-category ids carry
// no state suffix (normal); sub-category ids end in _Over.
func zoneIconRect(name string) (image.Rectangle, bool) {
	const prefix = "combine_etc_dropitem_icon_"
	low := strings.ToLower(name)
	i := strings.Index(low, prefix)
	if i < 0 {
		return image.Rectangle{}, false
	}
	rest := low[i+len(prefix):] // e.g. "tab_05" or "filter_btn_05_over"
	state := 0
	for _, s := range iconStates {
		if strings.HasSuffix(rest, s.suffix) {
			state, rest = s.idx, strings.TrimSuffix(rest, s.suffix)
			break
		}
	}
	j := strings.LastIndex(rest, "_")
	if j < 0 {
		return image.Rectangle{}, false
	}
	g, ok := zoneIconGrids[rest[:j]]
	num, err := strconv.Atoi(rest[j+1:])
	if !ok || err != nil || num < 1 {
		return image.Rectangle{}, false
	}
	x := g.x0 + (num-1)*g.hStride
	y := g.y0 + state*g.vStride
	return image.Rect(x, y, x+g.w, y+g.h), true
}

// zoneCategory crops each Monster Zone Info main/sub-category icon out of the single
// UI atlas and writes it as <iconId>.png under icons/zonecategories, where <iconId>
// is the icon field from zones.json. One DDS decode fans out into many PNG crops, so
// the source matches the atlas path and returns all crops from one Convert.
type zoneCategory struct {
	base
	names map[string]bool
}

func zoneCategoryIcons() imageSource {
	return &zoneCategory{}
}

func (s *zoneCategory) Name() string { return "zone-category icons" }
func (s *zoneCategory) Dir() string  { return "icons/zonecategories" }

func (s *zoneCategory) Prepare(src *paz.Source, dataDir string) error {
	s.src = src
	var zones []struct {
		MainCategory *struct {
			Icon string `json:"icon"`
		} `json:"mainCategory"`
		SubCategories []struct {
			Icon string `json:"icon"`
		} `json:"subCategories"`
	}
	if err := jsonio.ReadFile(filepath.Join(dataDir, "zones.json"), &zones); err != nil {
		return nil // zones.json not built yet — nothing to do
	}
	s.names = map[string]bool{}
	for _, z := range zones {
		if z.MainCategory != nil && z.MainCategory.Icon != "" {
			s.names[z.MainCategory.Icon] = true
		}
		for _, c := range z.SubCategories {
			if c.Icon != "" {
				s.names[c.Icon] = true
			}
		}
	}
	return nil
}

func (s *zoneCategory) Wants(path string) bool {
	return len(s.names) > 0 && strings.ToLower(path) == zoneCategoryAtlas
}

func (s *zoneCategory) Convert(path string, f paz.PazFile) ([]output, error) {
	dds, err := s.src.Archive.Content(f)
	if err != nil {
		return nil, err
	}
	atlas, err := tex.DecodeDDS(dds)
	if err != nil {
		return nil, err
	}
	// each icon has 3 state rows in the atlas: Normal (<id>.png, the resting bar
	// look), Over (<id>_Over.png) and Click (<id>_Click.png).
	var outs []output
	for name := range s.names {
		for _, suffix := range []string{"", "_Over", "_Click"} {
			rect, ok := zoneIconRect(name + suffix)
			if !ok {
				continue
			}
			sub, ok := atlas.SubImage(rect).(*image.NRGBA)
			if !ok || sub.Bounds().Empty() {
				continue
			}
			data := encodeIconImage(sub)
			if data == nil {
				continue
			}
			outs = append(outs, output{Rel: name + suffix + utils.IconExt, Data: data})
		}
	}
	return outs, nil
}
