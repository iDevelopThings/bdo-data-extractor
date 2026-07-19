// Command bdo-data-extractor is a single-entry CLI for reading Black Desert Online's
// PAZ game data (read-only) and decoding its .bss/.dbss tables — see README.md for subcommands and flags.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/build"
	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/pipeline"
	"github.com/idevelopthings/bdo-data-extractor/src/output"
)

func main() {
	conf, cmd, rest := config.InitConfig()

	var err error
	switch cmd {
	case "meta":
		err = cmdMeta(*conf.GameDir)
	case "extract":
		if len(rest) < 2 {
			config.DumpUsageAndExit()
		}
		err = cmdExtract(*conf.GameDir, rest[0], rest[1])
	case "build":
		err = build.Run()
	case "diff-outputs":
		if len(rest) < 2 {
			config.DumpUsageAndExit()
		}
		err = cmdDiffOutputs(rest[0], rest[1])
	case "icons":
		err = pipeline.Icons()
	case "index":
		err = pipeline.Index()
	case "maps":
		err = pipeline.Maps()
	case "regionmaps":
		err = pipeline.RegionMaps()
	case "worldmap":
		err = pipeline.WorldMap()
	case "knowledge-icons":
		err = pipeline.KnowledgeIcons()
	case "loc":
		err = pipeline.Loc()
	case "lua-strings":
		var out string
		if out, err = outFile(filepath.Join(*conf.Out, "lua_strings_"+*conf.Lang+".json"), rest); err == nil {
			err = pipeline.LuaStrings(out)
		}
	default:
		config.DumpUsageAndExit()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdMeta(gameDir string) error {
	ix, err := paz.LoadMeta(gameDir)
	if err != nil {
		return err
	}
	fmt.Printf(
		"version=%d paz_count=%d files=%d folders=%d filenames=%d\n",
		ix.Version, ix.PazCount, len(ix.Files), len(ix.FolderNames), len(ix.FileNames),
	)
	for i, f := range ix.Files {
		if i >= 6 {
			break
		}
		fmt.Printf("  %s  [paz%05d off=%d c=%d o=%d]\n", ix.PathOf(f), f.PazNumber, f.Offset, f.CompSize, f.OrigSize)
	}
	return nil
}

func cmdExtract(gameDir, substr, outDir string) error {
	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := src.Archive.AssertSafeOut(outDir); err != nil {
		return err
	}
	q := strings.ToLower(substr)
	n := 0
	for i, f := range src.Index.Files {
		p := src.Index.Path(i)
		if !strings.Contains(strings.ToLower(p), q) {
			continue
		}
		content, err := src.Archive.Content(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", p, err)
			continue
		}
		dest := filepath.Join(outDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return err
		}
		n++
	}
	fmt.Printf("extracted %d files -> %s\n", n, outDir)
	return nil
}

// cmdDiffOutputs compares two build dirs using each side's .build-outputs.json.
// Unowned assets (icons, tiles, provenance) are ignored.
func cmdDiffOutputs(leftDir, rightDir string) error {
	diff, err := output.DiffOwned(leftDir, rightDir)
	if err != nil {
		return err
	}

	fmt.Printf("owned files: %d identical\n", diff.Same)
	for _, name := range diff.OnlyLeft {
		fmt.Printf("  only in left:  %s\n", name)
	}
	for _, name := range diff.OnlyRight {
		fmt.Printf("  only in right: %s\n", name)
	}
	for _, c := range diff.Changed {
		fmt.Printf("  changed: %s  left=%dB(%s) right=%dB(%s)\n",
			c.Name, c.LeftSize, c.LeftSHA, c.RightSize, c.RightSHA)
	}

	if diff.Equal() {
		fmt.Println("outputs match")
		return nil
	}
	fmt.Fprintf(os.Stderr, "outputs differ: %d only-left, %d only-right, %d changed\n",
		len(diff.OnlyLeft), len(diff.OnlyRight), len(diff.Changed))
	return fmt.Errorf("build outputs differ")
}

// outFile resolves an output path (default, or rest[0] if given) and ensures its
// parent directory exists.
func outFile(defaultPath string, rest []string) (string, error) {
	p := defaultPath
	if len(rest) >= 1 {
		p = rest[0]
	}
	return p, os.MkdirAll(filepath.Dir(p), 0o755)
}
