package tables

import (
	"reflect"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// ingredient identity for comparison, independent of ItemRef internals.
type ic struct {
	id    uint32
	count int
}

func projectInputs(ins []model.Ingredient) []ic {
	out := make([]ic, len(ins))
	for i, in := range ins {
		out[i] = ic{id: in.Item.ID(), count: in.Count}
	}
	return out
}

// Alchemy/cook blocks encode quantity by repeating an item with no count attr;
// house blocks carry an explicit count. Both must fold to one entry per item with
// a summed count, so no consumer sees a duplicate or countless ingredient.
func TestParseItemInfoNormalizesIngredients(t *testing.T) {
	data := []byte(`<itemInfo>
		<itemKey>9715</itemKey>
		<alchemy>
			<item><id>5301</id></item>
			<item><id>5301</id></item>
			<item><id>4006</id></item>
			<item><id>4051</id></item>
		</alchemy>
		<house type="1">
			<item><id>6656</id><count>10</count></item>
			<item><id>4001</id><count>3</count></item>
		</house>
	</itemInfo>`)

	info := ParseItemInfo(data, nil)
	if info == nil {
		t.Fatal("ParseItemInfo returned nil")
	}
	if len(info.Recipes) != 2 {
		t.Fatalf("recipes = %d, want 2 (alchemy, house)", len(info.Recipes))
	}

	// Alchemy: the doubled 5301 collapses to a count of 2, the rest to 1.
	if got, want := projectInputs(info.Recipes[0].Inputs), []ic{
		{id: 5301, count: 2},
		{id: 4006, count: 1},
		{id: 4051, count: 1},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("alchemy inputs = %v, want %v", got, want)
	}

	// House: explicit counts pass through unchanged.
	if got, want := projectInputs(info.Recipes[1].Inputs), []ic{
		{id: 6656, count: 10},
		{id: 4001, count: 3},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("house inputs = %v, want %v", got, want)
	}
}
