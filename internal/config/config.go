package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
)

type Config struct {
	GameDir     *string
	Out         *string
	Lang        *string
	Pretty      *bool
	DumpItemIds []uint32 // optional list of item IDs to dump (for debugging)
}

var GlobalConfig *Config

// Set populates GlobalConfig programmatically. It is the embedder's entry point,
// mirroring what InitConfig does for the CLI, so the pipeline's single source of
// truth stays GlobalConfig whether it's driven by flags or by an embedding app.
func Set(gameDir, out, lang string, pretty bool) {
	GlobalConfig = &Config{
		GameDir: &gameDir,
		Out:     &out,
		Lang:    &lang,
		Pretty:  &pretty,
	}
}

func InitConfig() (*Config, string, []string) {
	GlobalConfig = &Config{}

	if len(os.Args) < 2 {
		DumpUsageAndExit()
	}
	cmd := os.Args[1]

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = usage

	GlobalConfig.GameDir = fs.String("game", paz.DefaultGameDir, "game install directory (read-only)")
	GlobalConfig.Out = fs.String("out", "data", "output directory for build")
	GlobalConfig.Lang = fs.String("lang", "en", "localization language (en/de/fr/sp)")
	GlobalConfig.Pretty = fs.Bool("pretty", false, "indent JSON output (build command)")

	fs.Func("dump-item-ids", "comma-separated list of item IDs to dump (for debugging)", func(s string) error {
		if s == "" {
			return nil
		}
		ids := strings.Split(s, ",")

		idList := make([]uint32, 0)
		for _, idStr := range ids {
			id, err := strconv.ParseUint(idStr, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid item ID %q: %v", idStr, err)
			}
			idList = append(idList, uint32(id))
		}
		if len(idList) > 0 {
			GlobalConfig.DumpItemIds = idList
		} else {
			GlobalConfig.DumpItemIds = nil
		}

		return nil
	})

	_ = fs.Parse(os.Args[2:])
	rest := fs.Args()

	return GlobalConfig, cmd, rest
}

func DumpUsageAndExit() {
	usage()
	os.Exit(2)
}

func usage() {
	_, err := fmt.Fprintln(
		os.Stderr,
		`bdo-data-extractor <command> [flags] [args]    (flags precede positional args)

  build [outPath]            collects all possible data, items, recipes, territories etc...
  meta                       parse pad00000.meta, print summary
  extract <substr> <outDir>  extract decoded files whose path contains substr
  table <name>               decode one table via a known schema -> JSON (stdout)
  icons [outDir]             decode each item's icon to <id>.png (default <out>/icons)
  loc [outPath]              dump the ENTIRE localization file (all tables) -> JSON
  regionmaps                 decode each region map to <out>/regionmaps/<map>.png
  worldmap                   decode the world map to <out>/worldmap.png
  knowledge-icons            decode each knowledge card's encyclopedia image to <out>/knowledge_icons/<image>

flags: --game DIR (game install, read-only)  --out DIR (output, default "data")
       --lang en|de|fr|sp   --pretty (indent JSON)`,
	)
	if err != nil {
		log.Fatalf("failed to write usage: %v", err)
		return
	}
}
