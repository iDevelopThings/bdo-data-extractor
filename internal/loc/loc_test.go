package loc

import "testing"

func TestSetAdventureJournalText(t *testing.T) {
	t.Parallel()

	texts := make(map[uint32]map[uint32]AdventureJournalText)
	for plane, text := range []string{"Journal", "Description", "Requirement", "Book"} {
		setAdventureJournalText(texts, 2, uint32(plane)<<24|7, text)
	}
	setAdventureJournalText(texts, 2, 4<<24|7, "ignored")

	got := texts[2][7]
	want := AdventureJournalText{
		JournalName:        "Journal",
		JournalDescription: "Description",
		Requirement:        "Requirement",
		Name:               "Book",
	}
	if got != want {
		t.Fatalf("journal text = %+v, want %+v", got, want)
	}
}

func TestSetNameDesc(t *testing.T) {
	t.Parallel()

	m := map[uint32]Text{}
	setNameDesc(m, 7, fieldName, "Name")
	setNameDesc(m, 7, fieldDesc, "Desc")
	setNameDesc(m, 7, fieldCol2, "ignored")
	if got := m[7]; got != (Text{Name: "Name", Description: "Desc"}) {
		t.Fatalf("text = %+v", got)
	}
}

func TestSetItemField(t *testing.T) {
	t.Parallel()

	items := map[uint32]ItemText{}
	setItemField(items, 1, fieldName, "Sword")
	setItemField(items, 1, fieldDesc, "Sharp")
	setItemField(items, 1, fieldCol2, "Use me")
	setItemField(items, 1, fieldCol3, "Trade me")
	want := ItemText{Name: "Sword", Description: "Sharp", Use: "Use me", Exchange: "Trade me"}
	if got := items[1]; got != want {
		t.Fatalf("item = %+v, want %+v", got, want)
	}
}

func TestSetBuffNameKeepsFullTextFirstWrite(t *testing.T) {
	t.Parallel()

	names := map[uint32]string{}
	setBuffName(names, 9, 0, "Title\n\nAll AP +10")
	setBuffName(names, 9, 0, "other")
	setBuffName(names, 9, 1, "wrong field")
	if got := names[9]; got != "Title\n\nAll AP +10" {
		t.Fatalf("buff = %q", got)
	}
}

func TestSetLightstoneText(t *testing.T) {
	t.Parallel()

	sets := map[uint32]Text{}
	setLightstoneText(sets, 3, "[Combo]\n<PAColor>effect</PAColor>")
	got := sets[3]
	if got.Name != "Combo" || got.Description != "[Combo]\n<PAColor>effect</PAColor>" {
		t.Fatalf("lightstone = %+v", got)
	}
}
