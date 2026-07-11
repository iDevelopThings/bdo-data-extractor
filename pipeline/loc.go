package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
)

// Loc dumps the entire localization file (all string tables) to JSON, grouped as
// table(key0) -> id -> field(key1) -> text. Game dir, language and output dir come
// from the global config.
func Loc() error {
	gameDir := *config.GlobalConfig.GameDir
	lang := *config.GlobalConfig.Lang
	outPath := filepath.Join(*config.GlobalConfig.Out, "loc_"+lang+".json")

	recs, err := loc.LoadAll(gameDir, lang)
	if err != nil {
		return err
	}

	grouped := make(map[uint32]map[uint32]map[uint32]string)
	counts := make(map[uint32]int)
	sample := make(map[uint32]string)
	for _, r := range recs {
		if grouped[r.Key0] == nil {
			grouped[r.Key0] = make(map[uint32]map[uint32]string)
		}
		if grouped[r.Key0][r.ID] == nil {
			grouped[r.Key0][r.ID] = make(map[uint32]string)
		}
		grouped[r.Key0][r.ID][r.Key1] = r.Text
		counts[r.Key0]++
		if sample[r.Key0] == "" && r.Text != "" && r.Text != "<null>" {
			sample[r.Key0] = r.Text
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := jsonio.WriteFile(outPath, grouped, true); err != nil {
		return err
	}

	keys := make([]uint32, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	progress.Default().Log(fmt.Sprintf("dumped %d strings across %d tables -> %s", len(recs), len(counts), outPath))

	return nil
}
