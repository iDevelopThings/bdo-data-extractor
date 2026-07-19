package output

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiffOwnedIdentical(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeOwned(t, dir, map[string]string{
		"a.json": `{"x":1}`,
		"b.json": `[]`,
	})

	diff, err := DiffOwned(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Equal() || diff.Same != 2 {
		t.Fatalf("diff = %+v", diff)
	}
}

func TestDiffOwnedReportsChanges(t *testing.T) {
	t.Parallel()

	left := t.TempDir()
	right := t.TempDir()
	writeOwned(t, left, map[string]string{
		"same.json":    `{"ok":true}`,
		"changed.json": `{"v":1}`,
		"gone.json":    `{}`,
	})
	writeOwned(t, right, map[string]string{
		"same.json":    `{"ok":true}`,
		"changed.json": `{"v":2}`,
		"new.json":     `{}`,
	})

	diff, err := DiffOwned(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Equal() {
		t.Fatal("expected differences")
	}
	if diff.Same != 1 {
		t.Fatalf("same = %d, want 1", diff.Same)
	}
	if len(diff.OnlyLeft) != 1 || diff.OnlyLeft[0] != "gone.json" {
		t.Fatalf("OnlyLeft = %v", diff.OnlyLeft)
	}
	if len(diff.OnlyRight) != 1 || diff.OnlyRight[0] != "new.json" {
		t.Fatalf("OnlyRight = %v", diff.OnlyRight)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].Name != "changed.json" {
		t.Fatalf("Changed = %+v", diff.Changed)
	}
}

func TestReadOwnershipManifestMissing(t *testing.T) {
	t.Parallel()
	if _, err := ReadOwnershipManifest(t.TempDir()); err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func writeOwned(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	names := make([]string, 0, len(files))
	for name, body := range files {
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest(t, filepath.Join(dir, manifestName), names)
}
