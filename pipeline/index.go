package pipeline

import (
	"log"
	"path/filepath"
	"slices"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
)

// Index purely a debug util to see what's in the indexes easily
func Index() error {
	gameDir := *config.GlobalConfig.GameDir
	dataDir := *config.GlobalConfig.Out

	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()

	skipExts := make([]string, 0)
	onlyExts := make([]string, 0)
	onlyDirs := make([]string, 0)

	if config.GlobalConfig.IgnoreExts != nil && *config.GlobalConfig.IgnoreExts != "" {
		skipExts = strings.Split(*config.GlobalConfig.IgnoreExts, ",")
	}
	if config.GlobalConfig.OnlyExts != nil && *config.GlobalConfig.OnlyExts != "" {
		onlyExts = strings.Split(*config.GlobalConfig.OnlyExts, ",")
	}
	if config.GlobalConfig.OnlyDirs != nil && *config.GlobalConfig.OnlyDirs != "" {
		onlyDirs = strings.Split(*config.GlobalConfig.OnlyDirs, ",")
	}

	log.Printf("Indexing %d files from %s", len(src.Index.Files), gameDir)
	log.Printf("Ignoring extensions: %v", skipExts)
	log.Printf("Including only extensions: %v", onlyExts)
	log.Printf("Including only directories: %v", onlyDirs)

	files := make([]string, 0)
	for _, file := range src.Index.Files {
		p := src.Index.PathOf(file)
		ext := strings.ToLower(filepath.Ext(p))
		if len(onlyExts) > 0 {
			if slices.IndexFunc(onlyExts, func(s string) bool { return s == ext }) < 0 {
				continue
			}
		}
		if len(skipExts) > 0 {
			if slices.IndexFunc(skipExts, func(s string) bool { return s == ext }) >= 0 {
				continue
			}
		}
		if len(onlyDirs) > 0 {
			dir := filepath.Dir(p)
			if slices.IndexFunc(onlyDirs, func(s string) bool { return strings.HasPrefix(dir, s) }) < 0 {
				continue
			}
		}

		files = append(files, p)
	}

	slices.SortFunc(files, func(a, b string) int {
		depthA := strings.Count(a, "/")
		depthB := strings.Count(b, "/")
		if depthA != depthB {
			return depthA - depthB
		}
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})

	if err := jsonio.WriteFile(filepath.Join(dataDir, "paz_files.json"), files, *config.GlobalConfig.Pretty); err != nil {
		return err
	}

	log.Printf("Wrote %d file names to paz_files.json", len(files))

	dirs := slices.Clone(src.Index.FolderNames)
	outDirs := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if len(onlyDirs) > 0 {
			if slices.IndexFunc(onlyDirs, func(s string) bool { return strings.HasPrefix(dir, s) }) < 0 {
				continue
			}
		}
		outDirs = append(outDirs, dir)
	}

	slices.SortFunc(outDirs, func(a, b string) int {
		depthA := strings.Count(a, "/")
		depthB := strings.Count(b, "/")
		if depthA != depthB {
			return depthA - depthB
		}
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})

	if err := jsonio.WriteFile(filepath.Join(dataDir, "paz_dirs.json"), outDirs, *config.GlobalConfig.Pretty); err != nil {
		return err
	}

	log.Printf("Wrote %d folder names to paz_dirs.json", len(outDirs))

	return nil
}
