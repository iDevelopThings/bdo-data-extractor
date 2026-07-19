package paz

import (
	"sync"
	"testing"
)

func TestIndexFindConcurrent(t *testing.T) {
	t.Parallel()

	ix := &Index{
		Files: []PazFile{
			{FileID: 0, FolderID: 0},
			{FileID: 1, FolderID: 0},
		},
		FolderNames: []string{"a/"},
		FileNames:   []string{"foo.dbss", "bar.bss"},
	}

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := ix.Find("foo.dbss"); !ok {
				t.Error("foo.dbss missing")
			}
			if _, ok := ix.Find("bar.bss"); !ok {
				t.Error("bar.bss missing")
			}
			_ = ix.FilesUnder("a")
		}()
	}
	wg.Wait()
}
