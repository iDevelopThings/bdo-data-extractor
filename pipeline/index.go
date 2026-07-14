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

	skipExts := []string{".ai", ".txt", ".paa", ".pae", ".vnl", ".bnk", ".paac", ".pac", ".xml"}
	files := make([]string, 0)
	for _, file := range src.Index.Files {
		p := src.Index.PathOf(file)

		if slices.ContainsFunc(skipExts, func(ext string) bool { return strings.HasSuffix(p, ext) }) {
			continue
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

	type DirData struct {
		InterestingDirs []string `json:"interesting_dirs"`
		Dirs            []string `json:"dirs"`
	}
	data := DirData{
		Dirs: slices.Clone(src.Index.FolderNames),
	}

	data.InterestingDirs = slices.DeleteFunc(slices.Clone(data.Dirs), func(s string) bool {
		return !strings.Contains(strings.ToLower(s), "map")
	})

	if err := jsonio.WriteFile(filepath.Join(dataDir, "paz_dirs.json"), data, *config.GlobalConfig.Pretty); err != nil {
		return err
	}

	log.Printf("Wrote %d folder names to paz_dirs.json", len(data.Dirs))

	return nil
}
