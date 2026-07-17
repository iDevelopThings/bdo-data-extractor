package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
)

// LuaStrings dumps symbolic PAGetString keys resolved through localization table 37.
func LuaStrings(outPath string) error {
	gameDir := *config.GlobalConfig.GameDir
	lang := *config.GlobalConfig.Lang
	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()

	data, err := src.Read("stringtable.bss")
	if err != nil {
		return fmt.Errorf("read stringtable.bss: %w", err)
	}
	catalog, err := loc.DecodeLuaStrings(data)
	if err != nil {
		return err
	}
	table, err := loc.LoadTable(gameDir, lang, loc.SymbolicStringTable)
	if err != nil {
		return fmt.Errorf("load localization table %d: %w", loc.SymbolicStringTable, err)
	}
	catalog.Resolve(table)

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := jsonio.WriteFile(outPath, catalog, *config.GlobalConfig.Pretty); err != nil {
		return err
	}

	entries, fallbacks := 0, 0
	for _, sheet := range catalog.Sheets {
		entries += len(sheet.Strings)
		for _, value := range sheet.Strings {
			if value.SourceFallback {
				fallbacks++
			}
		}
	}
	progress.Default().Log(fmt.Sprintf("dumped %d Lua strings across %d sheets (%d source fallbacks) -> %s", entries, len(catalog.Sheets), fallbacks, outPath))
	return nil
}
