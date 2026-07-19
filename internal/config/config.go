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
	GameDir *string
	Out     *string
	Lang    *string
	// Region selects the game service region, such as na.
	Region      *string
	Pretty      *bool
	DumpItemIds []uint32 // optional list of item IDs to dump (for debugging)

	// Index CMD related:
	IgnoreExts *string // comma-separated list of file extensions to ignore when indexing
	OnlyExts   *string // comma-separated list of file extensions to include when indexing (overrides IgnoreExts)
	OnlyDirs   *string // comma-separated list of directories to include when indexing (overrides IgnoreExts)
}

var GlobalConfig *Config

// Set populates GlobalConfig programmatically. It is the embedder's entry point,
// mirroring what InitConfig does for the CLI, so the pipeline's single source of
// truth stays GlobalConfig whether it's driven by flags or by an embedding app.
func Set(gameDir, out, lang, region string, pretty bool) {
	GlobalConfig = &Config{
		GameDir: &gameDir,
		Out:     &out,
		Lang:    &lang,
		Region:  &region,
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
	GlobalConfig.Region = fs.String("region", "", "game service region, e.g. na")
	GlobalConfig.Pretty = fs.Bool("pretty", false, "indent JSON output (build command)")

	// Index cmd related:
	GlobalConfig.IgnoreExts = fs.String("ignore-exts", ".ai,.txt,.paa,.pae,.vnl,.bnk,.paac,.pac,.xml", "comma-separated list of file extensions to ignore when indexing")
	GlobalConfig.OnlyExts = fs.String("only-exts", "", "comma-separated list of file extensions to include when indexing (overrides ignore-exts)")
	GlobalConfig.OnlyDirs = fs.String("only-dirs", "", "comma-separated list of directories to include when indexing (overrides ignore-exts)")

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
  diff-outputs <left> <right>
                             compare two build dirs via .build-outputs.json (owned files only)
  meta                       parse pad00000.meta, print summary
  extract <substr> <outDir>  extract decoded files whose path contains substr
  index <outDir> <(opt)ignore-exts> <(opt)only-exts> <(opt)only-dirs> dump the archive listing -> <out>/paz_files.json + paz_dirs.json
  icons [outDir]             decode each item's icon to <id>.webp (default <out>/icons)
  knowledge-icons            decode each knowledge card's encyclopedia image to <out>/knowledge_icons/<image>
  loc [outPath]              dump the ENTIRE localization file (all tables) -> JSON
  lua-strings [outPath]      dump PAGetString symbolic keys resolved through localization -> JSON
  worldmap                   decode the world map to a tile pyramid in <out>/worldmap/<layer>/
  maps                       same as worldmap (region masks are built by regionmaps)
  regionmaps                 decode each region map to <out>/regionmaps/<map>.png

flags: --game DIR (game install, read-only)  --out DIR (output, default "data")
       --lang en|de|fr|sp   --region na (game service region; default: language only)
       --pretty (indent JSON)`,
	)
	if err != nil {
		log.Fatalf("failed to write usage: %v", err)
		return
	}
}
