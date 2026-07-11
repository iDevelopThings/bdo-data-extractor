package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
)

// KnowledgeIcons decodes each knowledge card's encyclopedia image (a .dds in the
// PAZ) to <dataDir>/knowledge_icons/<image>, where <image> is the path stored in
// knowledge.json (run build first). Shared images decode once. Game dir and data
// dir come from the global config.
func KnowledgeIcons() error {
	gameDir := *config.GlobalConfig.GameDir
	dataDir := *config.GlobalConfig.Out
	outDir := filepath.Join(dataDir, "knowledge_icons")

	var k struct {
		Entries []struct {
			Key   uint32 `json:"key"`
			Image string `json:"image"`
		} `json:"entries"`
	}
	if err := jsonio.ReadFile(filepath.Join(dataDir, "knowledge.json"), &k); err != nil {
		return fmt.Errorf("load knowledge.json (run build first): %w", err)
	}

	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := src.Archive.AssertSafeOut(outDir); err != nil {
		return err
	}
	files := make(map[string]paz.PazFile)
	for i, f := range src.Index.Files {
		p := strings.ToLower(src.Index.Path(i))
		if strings.HasSuffix(p, ".dds") && strings.Contains(p, "ui_artwork/") {
			files[p] = f
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Card images are mostly shared category art, so decode + write each unique
	// image once. dest maps the PAZ source path -> the output relative path (the
	// knowledge.json `image`, a lowercase .png mirroring the source subdir).
	dest := map[string]string{}
	for _, e := range k.Entries {
		if e.Image != "" {
			dest["ui_texture/"+strings.TrimSuffix(e.Image, ".png")+".dds"] = e.Image
		}
	}

	total := int64(len(dest))
	step := total / 50
	if step < 1 {
		step = 1
	}

	tasks := make(chan string, len(dest))
	for p := range dest {
		tasks <- p
	}
	close(tasks)

	rep := progress.Default()
	var written, missing int64
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range tasks {
				data := encodeIcon(src.Archive, files[p])
				if data == nil {
					atomic.AddInt64(&missing, 1)
					continue
				}
				out := filepath.Join(outDir, filepath.FromSlash(dest[p]))
				if os.MkdirAll(filepath.Dir(out), 0o755) != nil {
					continue
				}
				if os.WriteFile(out, data, 0o644) == nil {
					if n := atomic.AddInt64(&written, 1); n%step == 0 {
						rep.Progress(n, total)
					}
				}
			}
		}()
	}
	wg.Wait()
	rep.Progress(atomic.LoadInt64(&written), total)
	rep.Log(fmt.Sprintf("wrote %d knowledge icons -> %s (%d unconvertible/missing)", written, outDir, missing))

	return nil
}
