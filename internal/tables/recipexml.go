package tables

import (
	"encoding/xml"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// The per-item recipe XMLs (ui_html/xml/<itemId>.xml) are <itemInfo> docs that
// list, for the item the file is named after, the recipes that PRODUCE it:
//   <cook>/<alchemy>      = cooking / alchemy ingredients
//   <manufacture action="MANUFACTURE_HEAT|GRIND|DRY|THINNING|COOK|...">
//                         = processing (the action attr is the recipe type)
//   <house type="N">      = worker-building (House Crafting) recipe, e.g. Jeweler;
//                           the type attr selects the workshop
// Each block holds <item><id>N</id>…</item> ingredient entries; several blocks
// of one kind = alternative ingredient sets (e.g. Grain Juice from any grain).
// The <house> blocks carry <count>; the others usually don't.

type xmlSection struct {
	Action string `xml:"action,attr"`
	Type   string `xml:"type,attr"` // <house type="N">: the workshop (eHouseIconType)
	Items  []struct {
		ID    uint32 `xml:"id"`
		Count int    `xml:"count"`
	} `xml:"item"`
}

type xmlCharBlock struct {
	Chars []struct {
		Name string `xml:"name"`
	} `xml:"character"`
}

type xmlNode struct {
	Region string `xml:"region,attr"`
}

type xmlItemInfo struct {
	ItemKey     uint32         `xml:"itemKey"`
	Cook        []xmlSection   `xml:"cook"`
	Alchemy     []xmlSection   `xml:"alchemy"`
	Manufacture []xmlSection   `xml:"manufacture"`
	House       []xmlSection   `xml:"house"`
	Shop        []xmlCharBlock `xml:"shop"`    // vendor NPCs that sell it
	Collect     []xmlCharBlock `xml:"collect"` // gather/collect sources
	Node        []xmlNode      `xml:"node"`    // gathering node regions
}

// ItemInfo is everything the build pulls from one item-info XML: the recipes that
// produce the item, plus its acquisition (vendors, gather sources, nodes).
type ItemInfo struct {
	Key          uint32
	Recipes      []model.Recipe
	Vendors      []string
	GatheredFrom []string
	GatherNodes  []string
}

// decodeValidXML strips invalid UTF-8 (some files carry it in the unused Korean
// name/desc fields; ids/tags are ASCII) and decodes the result into v.
func decodeValidXML(data []byte, v any) error {
	return xml.NewDecoder(strings.NewReader(strings.ToValidUTF8(string(data), ""))).Decode(v)
}

// ParseItemInfo decodes an item-info XML into the recipes that produce the item
// plus its acquisition (vendors / gather sources / nodes). houseName resolves a
// <house type="N"> attribute to its workshop name (loc table 123); pass nil to
// leave HOUSE recipes' Station empty. Returns nil if it isn't an item-info doc.
func ParseItemInfo(data []byte, houseName func(houseType int) string) *ItemInfo {
	var doc xmlItemInfo
	if err := decodeValidXML(data, &doc); err != nil || doc.ItemKey == 0 {
		return nil
	}

	var recipes []model.Recipe
	add := func(secs []xmlSection, defaultType string) {
		for _, s := range secs {
			typ := defaultType
			if s.Action != "" {
				typ = strings.TrimPrefix(s.Action, "MANUFACTURE_")
				// MANUFACTURE_ALCHEMY/COOK are "Simple Alchemy"/"Simple Cooking" done in
				// the Processing window — distinct from real Alchemy/Cooking (the
				// <alchemy>/<cook> blocks, done with a tool). Keep them apart.
				if typ == "ALCHEMY" || typ == "COOK" {
					typ = "SIMPLE_" + typ
				}
			}
			ins := ingredientsOf(s)
			if len(ins) == 0 {
				continue
			}
			r := model.Recipe{Output: model.ItemRef(doc.ItemKey), Type: typ, Inputs: ins}
			if defaultType == "HOUSE" && houseName != nil {
				if ht, err := strconv.Atoi(s.Type); err == nil {
					r.Station = houseName(ht)
				}
			}
			recipes = append(recipes, r)
		}
	}
	add(doc.Cook, "COOK")
	add(doc.Alchemy, "ALCHEMY")
	add(doc.Manufacture, "MANUFACTURE")
	add(doc.House, "HOUSE")

	info := &ItemInfo{Key: doc.ItemKey, Recipes: recipes}
	info.Vendors = charNames(doc.Shop)
	info.GatheredFrom = charNames(doc.Collect)
	for _, n := range doc.Node {
		if r := strings.TrimSpace(n.Region); r != "" {
			info.GatherNodes = utils.AppendUnique(info.GatherNodes, r)
		}
	}

	return info
}

// charNames collects the distinct, non-empty <character><name> values across the
// given blocks, preserving first-seen order.
func charNames(blocks []xmlCharBlock) []string {
	var out []string
	for _, b := range blocks {
		for _, c := range b.Chars {
			if n := strings.TrimSpace(c.Name); n != "" {
				out = utils.AppendUnique(out, n)
			}
		}
	}

	return out
}

// ingredientsOf collects a section's non-empty ingredient entries.
func ingredientsOf(s xmlSection) []model.Ingredient {
	ins := make([]model.Ingredient, 0, len(s.Items))
	for _, it := range s.Items {
		if it.ID != 0 {
			ins = append(ins, model.Ingredient{Item: model.ItemRef(it.ID), Count: it.Count})
		}
	}

	return ins
}

// itemmaking.xml palette: <nodeProduct>/<collect>/<fishing> list gathered raw
// materials (their <item> uses a key attr). ParseGatheredItems returns those ids
// as the raw-material *candidates*. The build then drops any that actually have a
// real production recipe (some processed items, e.g. Flax Fabric, are mis-listed
// under <collect>), so only true gathered leaves stay flagged. See build.flagGathered.
type makingSection struct {
	Items []struct {
		Key uint32 `xml:"key,attr"`
	} `xml:"item"`
}
type itemMakingDoc struct {
	NodeProduct makingSection `xml:"nodeProduct"`
	Collect     makingSection `xml:"collect"`
	Fishing     makingSection `xml:"fishing"`
}

func ParseGatheredItems(data []byte) []uint32 {
	var doc itemMakingDoc
	if err := decodeValidXML(data, &doc); err != nil {
		return nil
	}
	var out []uint32
	for _, s := range []makingSection{doc.NodeProduct, doc.Collect, doc.Fishing} {
		for _, it := range s.Items {
			if it.Key != 0 {
				out = append(out, it.Key)
			}
		}
	}
	return out
}
