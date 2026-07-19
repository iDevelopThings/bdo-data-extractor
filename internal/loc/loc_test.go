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
