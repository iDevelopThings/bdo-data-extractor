package build

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func (b *Builder) buildAdventureJournals() error {
	files, err := b.readFiles(
		"journalquestoffset.dbss",
		"journalquest.dbss",
		"allquestlist.bss",
		"quest.dbss",
	)
	if err != nil {
		return err
	}
	indexData, journalData, questListData, questData := files[0], files[1], files[2], files[3]
	b.questConditions, err = tables.DecodeQuestConditions(questListData, questData)
	if err != nil {
		return err
	}
	if err := b.buildQuestCatalog(); err != nil {
		return err
	}
	rows, familyRows, err := tables.DecodeAdventureJournals(indexData, journalData, questListData, questData)
	if err != nil {
		return err
	}

	journals := make([]model.AdventureJournal, 0, len(rows))
	bookCount := 0
	pageCount := 0
	journalBonusCount := 0
	for _, row := range rows {
		journal := model.AdventureJournal{
			Key:         row.Key,
			Name:        row.SourceName,
			Description: row.SourceDescription,
			Books:       make([]model.AdventureJournalBook, 0, len(row.Books)),
		}
		for _, sourceBook := range row.Books {
			localized := b.gs.AdventureJournals[row.Key][sourceBook.Key]
			book := model.AdventureJournalBook{
				Key:                sourceBook.Key,
				JournalName:        localizedOrSource(localized.JournalName, sourceBook.SourceJournalName),
				JournalDescription: localizedOrSource(localized.JournalDescription, sourceBook.SourceJournalDescription),
				Name:               localizedOrSource(localized.Name, sourceBook.SourceName),
				Requirement:        localizedOrSource(localized.Requirement, sourceBook.SourceRequirement),
				Icon:               sourceBook.Icon,
				Texture:            sourceBook.Texture,
				Unknown8:           sourceBook.Unknown8,
				Pages:              make([]model.AdventureJournalPage, 0, len(sourceBook.Pages)),
			}
			if len(journal.Books) == 0 {
				journal.Name = book.JournalName
				journal.Description = book.JournalDescription
			}
			for _, sourcePage := range sourceBook.Pages {
				quest := model.QuestRef{ID: fmt.Sprintf("%d-%d", sourcePage.Group, sourcePage.Index)}
				b.fillQuest(&quest)
				if quest.Name == "" {
					return fmt.Errorf("journalquest: page quest %s is absent from loc table 18", quest.ID)
				}
				page := model.AdventureJournalPage{Quest: quest, Bonus: sourcePage.Bonus}
				book.Pages = append(book.Pages, page)
				pageCount++
				if page.Bonus != nil {
					journalBonusCount++
				}
			}
			journal.Books = append(journal.Books, book)
			bookCount++
		}
		journals = append(journals, journal)
	}

	bonuses := make([]model.FamilyStatQuestReward, 0, len(familyRows))
	totals := make(map[model.FamilyStatType]model.FamilyStatBonus)
	for _, row := range familyRows {
		quest := model.QuestRef{ID: fmt.Sprintf("%d-%d", row.Group, row.Index)}
		b.fillQuest(&quest)
		if quest.Name == "" {
			return fmt.Errorf("family-stat reward quest %s is absent from loc table 18", quest.ID)
		}
		bonuses = append(bonuses, model.FamilyStatQuestReward{
			Quest:      quest,
			RewardSlot: row.RewardSlot,
			Bonus:      row.Bonus,
		})
		total := totals[row.Bonus.Type]
		total.Type = row.Bonus.Type
		total.Value += row.Bonus.Value
		total.Unit = row.Bonus.Unit
		totals[row.Bonus.Type] = total
	}

	totalBonuses := make([]model.FamilyStatBonus, 0, len(totals))
	for _, total := range totals {
		totalBonuses = append(totalBonuses, total)
	}
	sort.Slice(totalBonuses, func(i, j int) bool {
		return totalBonuses[i].Type < totalBonuses[j].Type
	})
	if _, err := b.addJSON("adventure_journals.json", model.AdventureJournalData{
		Journals:     journals,
		Bonuses:      bonuses,
		TotalBonuses: totalBonuses,
	}); err != nil {
		return err
	}
	b.logf(fmt.Sprintf("journalquest: %d journals, %d books, %d pages, %d family-stat rewards (%d on journal pages)", len(journals), bookCount, pageCount, len(bonuses), journalBonusCount))
	return nil
}

func (b *Builder) buildQuestCatalog() error {
	ids := make([]uint32, 0, len(b.questConditions))
	for id := range b.questConditions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	quests := make([]model.QuestRef, 0, len(ids))
	for _, id := range ids {
		quest := model.QuestRef{ID: fmt.Sprintf("%d-%d", uint16(id), uint16(id>>16))}
		b.fillQuest(&quest)
		quests = append(quests, quest)
	}
	if _, err := b.addJSON("quests.json", quests); err != nil {
		return err
	}
	b.logf(fmt.Sprintf("quest: %d quests with client condition records", len(quests)))
	return nil
}

func localizedOrSource(localized, source string) string {
	if localized != "" {
		return localized
	}
	return source
}
