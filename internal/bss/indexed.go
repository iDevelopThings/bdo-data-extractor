package bss

import (
	"fmt"
	"iter"
)

// IndexedRecord is one offset-index entry paired with its sliced data bytes.
type IndexedRecord struct {
	Entry IndexEntry
	Data  []byte
}

// IndexedRecords yields each record from a 12-byte (key/offset/size) index.
// A parse failure or out-of-bounds slice yields a non-nil error and stops.
func IndexedRecords(offsetRaw, data []byte) iter.Seq2[IndexedRecord, error] {
	return func(yield func(IndexedRecord, error) bool) {
		entries, err := ParseOffsetIndex(offsetRaw, len(data))
		if err != nil {
			yield(IndexedRecord{}, err)
			return
		}
		recordsFromEntries(entries, data)(yield)
	}
}

// IndexedRecordsU16 yields each record from a [u16 key, u32 offset, u32 size] index.
// name labels skip/error logs from ParseU16OffsetIndex.
func IndexedRecordsU16(name string, offsetRaw, data []byte) iter.Seq2[IndexedRecord, error] {
	return func(yield func(IndexedRecord, error) bool) {
		entries, err := ParseU16OffsetIndex(name, offsetRaw, len(data))
		if err != nil {
			yield(IndexedRecord{}, err)
			return
		}
		recordsFromEntries(entries, data)(yield)
	}
}

// RecordsFromEntries yields sliced records for a pre-parsed index. Prefer
// IndexedRecords / IndexedRecordsU16 when the index still needs parsing.
func RecordsFromEntries(entries []IndexEntry, data []byte) iter.Seq2[IndexedRecord, error] {
	return recordsFromEntries(entries, data)
}

func recordsFromEntries(entries []IndexEntry, data []byte) iter.Seq2[IndexedRecord, error] {
	return func(yield func(IndexedRecord, error) bool) {
		for _, entry := range entries {
			rec, ok := entry.Slice(data)
			if !ok {
				yield(IndexedRecord{Entry: entry}, fmt.Errorf("key %d: invalid indexed slice", entry.Key))
				return
			}
			if !yield(IndexedRecord{Entry: entry, Data: rec}, nil) {
				return
			}
		}
	}
}
