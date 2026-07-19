package model

// AdventureJournalData is the client's adventure bookshelf and its permanent
// family-wide stat rewards.
type AdventureJournalData struct {
	Journals     []AdventureJournal      `json:"journals"`
	Bonuses      []FamilyStatQuestReward `json:"bonuses"`
	TotalBonuses []FamilyStatBonus       `json:"totalBonuses"`
}

// AdventureJournal is one bookshelf category containing one or more books.
type AdventureJournal struct {
	Key         uint32                 `json:"key"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Books       []AdventureJournalBook `json:"books"`
}

// AdventureJournalBook is one volume and its ordered pages.
type AdventureJournalBook struct {
	Key                uint32                 `json:"key"`
	JournalName        string                 `json:"journalName"`
	JournalDescription string                 `json:"journalDescription,omitempty"`
	Name               string                 `json:"name"`
	Requirement        string                 `json:"requirement,omitempty"`
	Icon               string                 `json:"icon,omitempty"`
	Texture            string                 `json:"texture,omitempty"`
	Unknown8           bool                   `json:"unknown8"`
	Pages              []AdventureJournalPage `json:"pages"`
}

// AdventureJournalPage is one quest-backed journal objective.
type AdventureJournalPage struct {
	Quest QuestRef         `json:"quest"`
	Bonus *FamilyStatBonus `json:"bonus,omitempty"`
}

// FamilyStatBonus is one permanent account-wide stat reward.
type FamilyStatBonus struct {
	Type  FamilyStatType `json:"type"`
	Value float64        `json:"value"`
	Unit  string         `json:"unit,omitempty"`
}

// FamilyStatQuestReward identifies the quest and reward slot granting a
// permanent account-wide stat.
type FamilyStatQuestReward struct {
	Quest      QuestRef        `json:"quest"`
	RewardSlot uint8           `json:"rewardSlot"`
	Bonus      FamilyStatBonus `json:"bonus"`
}
