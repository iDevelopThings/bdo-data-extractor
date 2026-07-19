package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

type benchmarkRecord struct {
	ID          uint32           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Icon        string           `json:"icon"`
	Grade       string           `json:"grade"`
	Prices      [3]int64         `json:"prices"`
	Flags       []string         `json:"flags,omitempty"`
	Stats       []benchmarkStat  `json:"stats,omitempty"`
	Attributes  map[string]int32 `json:"attributes,omitempty"`
}

type benchmarkStat struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}

func BenchmarkJSONDrivers(b *testing.B) {
	records := benchmarkRecords(16_384)
	sidecar := struct {
		Version int               `json:"version"`
		Groups  []benchmarkRecord `json:"groups"`
	}{Version: 1, Groups: records[:256]}

	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "Sidecar", value: sidecar},
		{name: "BulkArray", value: NewJSONArray(records)},
	} {
		b.Run(test.name, func(b *testing.B) {
			for _, driver := range []struct {
				name   string
				driver Driver
			}{
				{name: "encoding-json", driver: NewStandardJSONDriver(false)},
				{name: "goccy-go-json", driver: NewGoccyJSONDriver(false)},
				{name: "jettison", driver: NewJettisonJSONDriver(false)},
			} {
				b.Run(driver.name, func(b *testing.B) {
					benchmarkJSONDriver(b, driver.driver, test.value)
				})
			}
		})
	}
}

// BenchmarkJSONDriversExtractedItems benchmarks the real generated item model
// when BDOEXTRACT_BENCH_ITEMS names an items.json file. It is optional so the
// normal test suite does not depend on a local game extraction.
func BenchmarkJSONDriversExtractedItems(b *testing.B) {
	path := os.Getenv("BDOEXTRACT_BENCH_ITEMS")
	if path == "" {
		b.Skip("set BDOEXTRACT_BENCH_ITEMS to an extracted items.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var items []model.Item
	if err := json.Unmarshal(data, &items); err != nil {
		b.Fatal(err)
	}
	value := NewJSONArray(items)
	for _, driver := range []struct {
		name   string
		driver Driver
	}{
		{name: "encoding-json", driver: NewStandardJSONDriver(false)},
		{name: "goccy-go-json", driver: NewGoccyJSONDriver(false)},
		{name: "jettison", driver: NewJettisonJSONDriver(false)},
	} {
		b.Run(driver.name, func(b *testing.B) {
			benchmarkJSONDriver(b, driver.driver, value)
		})
	}
}

func benchmarkJSONDriver(b *testing.B, driver Driver, value any) {
	b.Helper()
	path := filepath.Join(b.TempDir(), "output.json")
	if err := driver.Write(path, value); err != nil {
		b.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(info.Size())
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := driver.Write(path, value); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkRecords(count int) []benchmarkRecord {
	records := make([]benchmarkRecord, count)
	for i := range records {
		records[i] = benchmarkRecord{
			ID:          uint32(i + 1),
			Name:        "Black Desert benchmark item",
			Description: "A representative nested item description with <markup> & symbols.",
			Icon:        "icons/00012345.webp",
			Grade:       "yellow",
			Prices:      [3]int64{1_000_000, 2_500_000, 7_500_000},
			Flags:       []string{"marketable", "enhanceable", "repairable"},
			Stats: []benchmarkStat{
				{Name: "AP", Value: 125},
				{Name: "Accuracy", Value: 47},
				{Name: "Critical Hit Rate", Value: 5, Unit: "%"},
			},
			Attributes: map[string]int32{"slot": 14, "kind": 15, "equipType": 7},
		}
	}
	return records
}
