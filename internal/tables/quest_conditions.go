package tables

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

const questConditionHeaderSize = 25

// QuestConditionRow is the client-evaluated acceptance and completion DSL for
// one quest record.
type QuestConditionRow struct {
	AcceptDSL              string
	CompleteDSL            string
	SourceObjective        string
	UnknownHeader          [25]byte
	UnknownObjectivePrefix []byte
	UnknownEnd             uint32
}

// DecodeQuestConditions reads the condition tail shared by ordinary quest.dbss
// records. allquestlist.bss supplies their physical order because quest.dbss has
// no separate offset index.
func DecodeQuestConditions(questListData, questData []byte) (map[uint32]QuestConditionRow, error) {
	questIDs, count, err := decodeOrderedQuestIDs(questListData)
	if err != nil {
		return nil, err
	}
	if len(questData) < 4 || int(bss.U32(questData, 0)) != count {
		return nil, fmt.Errorf("quest: row count does not match allquestlist count %d", count)
	}

	starts := make([]int, len(questIDs))
	from := 4
	for i, id := range questIDs {
		pos := indexQuestID(questData, from, id)
		if pos < 0 {
			return nil, fmt.Errorf("quest: ordered quest %d-%d is absent after offset %d", uint16(id), uint16(id>>16), from)
		}
		starts[i] = pos
		from = pos + 4
	}

	rows := make(map[uint32]QuestConditionRow, len(questIDs)-1)
	for i, id := range questIDs {
		end := len(questData)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		row, ok := decodeQuestConditionTail(questData[starts[i]:end], id)
		if !ok {
			// The final client row is an intentionally incomplete placeholder. All
			// indexed records before it must carry the normal condition tail.
			if i != len(questIDs)-1 {
				return nil, fmt.Errorf("quest: quest %d-%d has no condition tail", uint16(id), uint16(id>>16))
			}
			continue
		}
		rows[id] = row
	}
	return rows, nil
}

func decodeOrderedQuestIDs(data []byte) ([]uint32, int, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, 0, fmt.Errorf("allquestlist: %w", err)
	}
	recordSize, fixed := pabr.RecordSize()
	if !fixed || recordSize != 4 {
		return nil, 0, fmt.Errorf("allquestlist: record size %d, want 4", recordSize)
	}
	ids := make([]uint32, pabr.Rows)
	seen := make(map[uint32]bool, pabr.Rows)
	for row := range pabr.Rows {
		id := bss.U32(data, pabr.RecordsStart+row*recordSize)
		if id == 0 || seen[id] {
			return nil, 0, fmt.Errorf("allquestlist: invalid or duplicate quest id %08x", id)
		}
		seen[id] = true
		ids[row] = id
	}
	return ids, pabr.Rows, nil
}

func indexQuestID(data []byte, from int, id uint32) int {
	if from < 0 || from > len(data) {
		return -1
	}
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], id)
	rel := bytes.Index(data[from:], raw[:])
	if rel < 0 {
		return -1
	}
	return from + rel
}

func decodeQuestConditionTail(record []byte, id uint32) (QuestConditionRow, bool) {
	var found QuestConditionRow
	matches := 0
	for pos := 1; pos+4 <= len(record); pos++ {
		if bss.U32(record, pos) != id {
			continue
		}
		c := bss.NewCursor(record, pos, len(record))
		if c.U32() != id {
			continue
		}
		header := c.Bytes(questConditionHeaderSize)
		accept := c.UTF16()
		complete := c.UTF16()
		if !c.OK() {
			continue
		}
		conditionEnd := c.Pos()
		for objectivePos := conditionEnd + 24; objectivePos+12 <= len(record); objectivePos++ {
			if bss.PeekUTF16Chars(record, objectivePos) == 0 {
				continue
			}
			end := bss.NewCursor(record, objectivePos, len(record))
			objective := end.UTF16()
			unknownEnd := end.U32()
			if !end.OK() || end.Remaining() != 0 {
				continue
			}
			found = QuestConditionRow{
				AcceptDSL:              accept,
				CompleteDSL:            complete,
				SourceObjective:        objective,
				UnknownObjectivePrefix: bytes.Clone(record[conditionEnd:objectivePos]),
				UnknownEnd:             unknownEnd,
			}
			copy(found.UnknownHeader[:], header)
			matches++
			break
		}
	}
	return found, matches == 1
}
