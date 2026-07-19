package output

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

type testWriter func(string) error

type customJSON string

func (v customJSON) MarshalJSON() ([]byte, error) {
	return []byte(`{"custom":"` + string(v) + `"}`), nil
}

func TestRegisterRejectsUnsafeAndDuplicatePaths(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	value := testWriter(func(string) error { return nil })
	for _, name := range []string{"", ".", "..", filepath.Join("..", "items.json"), manifestName, journalName} {
		if err := tx.Register(name, value); err == nil {
			t.Errorf("Register(%q) error = nil", name)
		}
	}
	if err := tx.Register("items.json", value); err != nil {
		t.Fatalf("Register(items.json): %v", err)
	}
	if err := tx.Register("items.json", value); err == nil {
		t.Fatal("duplicate Register(items.json) error = nil")
	}
}

func TestRegisterDefersDriver(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	called := false
	if err := tx.Register("items.json", testWriter(func(string) error {
		called = true
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("driver ran during registration")
	}
}

func TestCustomAndJSONDrivers(t *testing.T) {
	t.Parallel()

	custom, err := New(t.TempDir(), DriverFunc(func(path string, value any) error {
		return os.WriteFile(path, []byte(value.(string)), 0o644)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := custom.Register("custom.txt", "custom"); err != nil {
		t.Fatal(err)
	}
	if err := custom.Publish(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(custom.dir, "custom.txt"), "custom")

	jsonTx, err := New(t.TempDir(), NewStandardJSONDriver(false))
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonTx.Register("object.json", map[string]int{"value": 7}); err != nil {
		t.Fatal(err)
	}
	if err := jsonTx.RegisterExclusive("array.json", NewJSONArray([]int{1, 2, 3})); err != nil {
		t.Fatal(err)
	}
	if err := jsonTx.Publish(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(jsonTx.dir, "object.json"), "{\"value\":7}\n")
	assertFile(t, filepath.Join(jsonTx.dir, "array.json"), "[1,2,3]\n")
}

func TestJSONDriversProduceEquivalentOutput(t *testing.T) {
	t.Parallel()

	type record struct {
		ID    int      `json:"id"`
		Name  string   `json:"name"`
		Tags  []string `json:"tags,omitempty"`
		Value float64  `json:"value"`
	}
	records := make([]record, 5000) // exercises the parallel-array path
	for i := range records {
		records[i] = record{ID: i, Name: "<item>&", Tags: []string{"a", "b"}, Value: float64(i) / 3}
	}

	for _, pretty := range []bool{false, true} {
		pretty := pretty
		t.Run(map[bool]string{false: "compact", true: "pretty"}[pretty], func(t *testing.T) {
			t.Parallel()

			standard := NewStandardJSONDriver(pretty)
			for _, value := range []any{
				map[string]any{
					"records": records[:10], "enabled": true,
					"marshaler": customJSON("used"), "scientific": 7.068285006514916e-7,
				},
				NewJSONArray(records),
			} {
				standardPath := filepath.Join(t.TempDir(), "standard.json")
				if err := standard.Write(standardPath, value); err != nil {
					t.Fatal(err)
				}
				standardData, err := os.ReadFile(standardPath)
				if err != nil {
					t.Fatal(err)
				}
				for _, driver := range []struct {
					name   string
					driver Driver
				}{
					{name: "goccy", driver: NewGoccyJSONDriver(pretty)},
					{name: "jettison", driver: NewJettisonJSONDriver(pretty)},
				} {
					path := filepath.Join(t.TempDir(), driver.name+".json")
					if err := driver.driver.Write(path, value); err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					assertJSONEquivalent(t, standardData, data)
					if !strings.Contains(string(data), "<item>&") {
						t.Fatalf("%s escaped HTML-sensitive item text", driver.name)
					}
				}
			}
		})
	}
}

func assertJSONEquivalent(t *testing.T, wantData, gotData []byte) {
	t.Helper()
	var want, got any
	if err := json.Unmarshal(wantData, &want); err != nil {
		t.Fatalf("decode reference JSON: %v", err)
	}
	if err := json.Unmarshal(gotData, &got); err != nil {
		t.Fatalf("decode driver JSON: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("driver produced different JSON data")
	}
}

func TestWriteBatchBoundsConcurrency(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	const count = 8
	entered := make(chan struct{}, count)
	release := make(chan struct{})
	artifacts := make([]artifact, count)
	var active atomic.Int32
	var maximum atomic.Int32
	for i := range artifacts {
		artifacts[i] = artifact{
			name: filepath.Join("nested", string(rune('a'+i))+".json"),
			value: testWriter(func(string) error {
				current := active.Add(1)
				for {
					old := maximum.Load()
					if current <= old || maximum.CompareAndSwap(old, current) {
						break
					}
				}
				entered <- struct{}{}
				<-release
				active.Add(-1)
				return nil
			}),
		}
	}

	done := make(chan error, 1)
	staging := t.TempDir()
	go func() {
		done <- tx.writeBatch(staging, artifacts, 2)
	}()
	<-entered
	<-entered
	select {
	case <-entered:
		t.Fatal("more than two artifact drivers entered concurrently")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := maximum.Load(); got != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", got)
	}
}

func TestWriteArtifactsRunsExclusiveDriversAlone(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	var active atomic.Int32
	var exclusiveActive atomic.Bool
	normal := testWriter(func(string) error {
		if exclusiveActive.Load() {
			return errors.New("normal driver overlapped exclusive driver")
		}
		active.Add(1)
		active.Add(-1)
		return nil
	})
	exclusive := testWriter(func(string) error {
		if active.Load() != 0 || !exclusiveActive.CompareAndSwap(false, true) {
			return errors.New("exclusive driver overlapped another driver")
		}
		exclusiveActive.Store(false)
		return nil
	})
	tx.artifacts = []artifact{
		{name: "before-a.json", value: normal},
		{name: "before-b.json", value: normal},
		{name: "items.json", exclusive: true, value: exclusive},
		{name: "after-a.json", value: normal},
		{name: "item_enhancements.json", exclusive: true, value: exclusive},
		{name: "after-b.json", value: normal},
	}
	if err := tx.writeArtifacts(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestPublishWriteFailurePreservesExistingData(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	writeFile(t, filepath.Join(tx.dir, "items.json"), "old items")
	if err := tx.Register("items.json", testWriter(func(path string) error {
		return os.WriteFile(path, []byte("new items"), 0o644)
	})); err != nil {
		t.Fatal(err)
	}
	if err := tx.Register("world.json", testWriter(func(string) error {
		return errors.New("injected write failure")
	})); err != nil {
		t.Fatal(err)
	}
	if err := tx.Publish(); err == nil {
		t.Fatal("Publish error = nil")
	}
	assertFile(t, filepath.Join(tx.dir, "items.json"), "old items")
	assertNotExist(t, filepath.Join(tx.dir, "world.json"))
	assertNoTransactionFiles(t, tx.dir)
}

func TestPublishOwnsOnlyManifestFiles(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	writeOldDataset(t, tx.dir)
	writeFile(t, filepath.Join(tx.dir, "notes.txt"), "user data")
	staging := writeNewDataset(t, tx.dir)
	if err := tx.publishStaged(staging, []string{"a.json", "c.json"}); err != nil {
		t.Fatal(err)
	}
	assertNewDataset(t, tx.dir)
	assertFile(t, filepath.Join(tx.dir, "notes.txt"), "user data")
	assertNoTransactionFiles(t, tx.dir)
}

func TestPublishWithoutPreviousManifestPreservesUnknownFiles(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	writeFile(t, filepath.Join(tx.dir, "legacy.json"), "legacy")
	staging, err := os.MkdirTemp(tx.dir, stagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(staging, "items.json"), "new items")
	writeManifest(t, filepath.Join(staging, manifestName), []string{"items.json"})
	if err := tx.publishStaged(staging, []string{"items.json"}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(tx.dir, "legacy.json"), "legacy")
	assertFile(t, filepath.Join(tx.dir, "items.json"), "new items")
}

func TestPublishRejectsIncompleteStagingBeforeJournal(t *testing.T) {
	t.Parallel()

	tx := newTestTransaction(t)
	writeOldDataset(t, tx.dir)
	staging, err := os.MkdirTemp(tx.dir, stagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	writeManifest(t, filepath.Join(staging, manifestName), []string{"a.json"})
	if err := tx.publishStaged(staging, []string{"a.json"}); err == nil {
		t.Fatal("publishStaged error = nil for missing staged artifact")
	}
	assertOldDataset(t, tx.dir)
	assertNotExist(t, filepath.Join(tx.dir, journalName))
}

func TestRecoverAtEveryPublishPoint(t *testing.T) {
	t.Parallel()

	points := collectPublishPoints(t)
	if !slices.Contains(points, "committed") {
		t.Fatal("publish points do not contain committed marker")
	}
	for _, point := range points {
		point := point
		t.Run(point, func(t *testing.T) {
			t.Parallel()

			tx := newTestTransaction(t)
			writeOldDataset(t, tx.dir)
			writeFile(t, filepath.Join(tx.dir, "notes.txt"), "user data")
			staging := writeNewDataset(t, tx.dir)
			interrupt := errors.New("simulated process interruption")
			tx.publishHook = func(got string) error {
				if got == point {
					return interrupt
				}
				return nil
			}
			err := tx.publishStaged(staging, []string{"a.json", "c.json"})
			if !errors.Is(err, interrupt) {
				t.Fatalf("publishStaged error = %v, want interruption at %q", err, point)
			}
			tx.publishHook = nil
			if err := tx.Recover(); err != nil {
				t.Fatalf("Recover: %v", err)
			}
			if point == "committed" {
				assertNewDataset(t, tx.dir)
			} else {
				assertOldDataset(t, tx.dir)
			}
			assertFile(t, filepath.Join(tx.dir, "notes.txt"), "user data")
			assertNoTransactionFiles(t, tx.dir)
		})
	}
}

func newTestTransaction(t *testing.T) *Transaction {
	t.Helper()
	tx, err := New(t.TempDir(), DriverFunc(func(path string, value any) error {
		return value.(testWriter)(path)
	}))
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func collectPublishPoints(t *testing.T) []string {
	t.Helper()
	tx := newTestTransaction(t)
	writeOldDataset(t, tx.dir)
	staging := writeNewDataset(t, tx.dir)
	var points []string
	tx.publishHook = func(point string) error {
		points = append(points, point)
		return nil
	}
	if err := tx.publishStaged(staging, []string{"a.json", "c.json"}); err != nil {
		t.Fatal(err)
	}
	return points
}

func writeOldDataset(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "a.json"), "old a")
	writeFile(t, filepath.Join(dir, "b.json"), "old b")
	writeManifest(t, filepath.Join(dir, manifestName), []string{"a.json", "b.json"})
}

func writeNewDataset(t *testing.T, dir string) string {
	t.Helper()
	staging, err := os.MkdirTemp(dir, stagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(staging, "a.json"), "new a")
	writeFile(t, filepath.Join(staging, "c.json"), "new c")
	writeManifest(t, filepath.Join(staging, manifestName), []string{"a.json", "c.json"})
	return staging
}

func writeManifest(t *testing.T, path string, files []string) {
	t.Helper()
	if err := writeNewJSON(path, manifest{Version: 1, Files: files}); err != nil {
		t.Fatal(err)
	}
}

func assertOldDataset(t *testing.T, dir string) {
	t.Helper()
	assertFile(t, filepath.Join(dir, "a.json"), "old a")
	assertFile(t, filepath.Join(dir, "b.json"), "old b")
	assertNotExist(t, filepath.Join(dir, "c.json"))
	tx := &Transaction{dir: dir}
	m, err := tx.readManifest(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Files, []string{"a.json", "b.json"}) {
		t.Fatalf("old manifest files = %v", m.Files)
	}
}

func assertNewDataset(t *testing.T, dir string) {
	t.Helper()
	assertFile(t, filepath.Join(dir, "a.json"), "new a")
	assertNotExist(t, filepath.Join(dir, "b.json"))
	assertFile(t, filepath.Join(dir, "c.json"), "new c")
	tx := &Transaction{dir: dir}
	m, err := tx.readManifest(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Files, []string{"a.json", "c.json"}) {
		t.Fatalf("new manifest files = %v", m.Files)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(b) != want {
		t.Fatalf("%s = %q, want %q", path, b, want)
	}
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists or returned unexpected error: %v", path, err)
	}
}

func assertNoTransactionFiles(t *testing.T, dir string) {
	t.Helper()
	assertNotExist(t, filepath.Join(dir, journalName))
	assertNotExist(t, filepath.Join(dir, committedName))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() && (strings.HasPrefix(entry.Name(), stagingPrefix) || strings.HasPrefix(entry.Name(), rollbackPrefix)) {
			t.Fatalf("transaction directory remains: %s", entry.Name())
		}
	}
}
