// Command bdo-data-extractor is a single-entry CLI for reading Black Desert Online's
// PAZ game data (read-only) and decoding its .bss/.dbss tables — see README.md for subcommands and flags.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/internal/build"
	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/schema"
	"github.com/idevelopthings/bdo-data-extractor/pipeline"
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
	case "table":
		if len(rest) < 1 {
			config.DumpUsageAndExit()
		}
		err = cmdTable(*conf.GameDir, *conf.Out, rest[0])
	case "build":
		// var out string
		// if out, err = outFile(filepath.Join(*conf.Out, "items.json"), rest); err == nil {
		err = build.Run()
		// }
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

func cmdTable(gameDir, outDir, name string) error {
	sc := schema.Registry[strings.ToLower(name)]
	if sc == nil {
		return fmt.Errorf("no schema registered for %q (known: %v)", name, knownSchemas())
	}
	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()
	content, _, err := src.ReadAny(name+".bss", name+".dbss")
	if err != nil {
		return fmt.Errorf("table %q: %w", name, err)
	}
	rows, err := bss.NewReader(content).ReadAll(sc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v (decoded %d rows)\n", err, len(rows))
	}
	fmt.Printf("decoded %d rows from %s\n", len(rows), name)
	show := rows
	if len(show) > 5 {
		show = show[:5]
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(show)
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

func knownSchemas() []string {
	var ks []string
	for k := range schema.Registry {
		ks = append(ks, k)
	}
	return ks
}
