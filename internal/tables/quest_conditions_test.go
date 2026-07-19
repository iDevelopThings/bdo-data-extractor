package tables

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDecodeQuestConditions(t *testing.T) {
	const (
		first  = uint32(1<<16 | 895)
		second = uint32(2<<16 | 895)
	)
	list := testQuestList(first, second)
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, 2)
	data = append(data, make([]byte, 9)...)
	data = append(data, testQuestConditionRecord(first, "ClearQuest(4501,3);", "meet(50493,1);", "Ask Hakan;")...)
	data = append(data, testQuestConditionRecord(second, "getLevel()>56;", "meet(40001,1);", "Talk to Islin;")...)

	got, err := DecodeQuestConditions(list, data)
	if err != nil {
		t.Fatal(err)
	}
	if got[first].AcceptDSL != "ClearQuest(4501,3);" || got[first].CompleteDSL != "meet(50493,1);" {
		t.Fatalf("first conditions = %+v", got[first])
	}
	if got[second].AcceptDSL != "getLevel()>56;" || got[second].CompleteDSL != "meet(40001,1);" {
		t.Fatalf("second conditions = %+v", got[second])
	}
	if got[first].SourceObjective != "Ask Hakan;" || len(got[first].UnknownObjectivePrefix) != 24 || got[first].UnknownEnd != 3 {
		t.Fatalf("first condition tail = %+v", got[first])
	}
}

func testQuestList(ids ...uint32) []byte {
	records := make([]byte, len(ids)*4)
	for i, id := range ids {
		binary.LittleEndian.PutUint32(records[i*4:], id)
	}
	return testPABRWithUTF16Strings(uint32(len(ids)), records)
}

func testQuestConditionRecord(id uint32, accept, complete, objective string) []byte {
	var record bytes.Buffer
	binary.Write(&record, binary.LittleEndian, id)
	record.Write(make([]byte, 64))
	binary.Write(&record, binary.LittleEndian, id)
	record.Write(make([]byte, questConditionHeaderSize))
	writeUTF16(&record, accept)
	writeUTF16(&record, complete)
	record.Write(make([]byte, 24))
	writeUTF16(&record, objective)
	binary.Write(&record, binary.LittleEndian, uint32(3))
	return record.Bytes()
}
