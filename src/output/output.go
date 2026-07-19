// Package output publishes groups of generated files as one recoverable transaction.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
)

// Transaction metadata uses reserved dot-prefixed names in the output root.
// Scratch directories live there too, keeping every rename on the same volume.
const (
	manifestName      = ".build-outputs.json"
	journalName       = ".build-transaction.json"
	committedName     = ".build-transaction.committed"
	stagingPrefix     = ".build-staging-"
	rollbackPrefix    = ".build-rollback-"
	markerPrefix      = ".build-marker-"
	defaultMaxWriters = 2
)

// Transaction collects output artifacts and publishes them without exposing a
// partially generated dataset. A Transaction is not safe for concurrent use.
type Transaction struct {
	dir        string
	driver     Driver
	artifacts  []artifact
	maxWriters int

	// publishHook is used by interruption tests to stop after a filesystem move.
	publishHook func(string) error
}

type artifact struct {
	name      string
	exclusive bool
	value     any
}

// manifest is the durable ownership boundary: only listed files may be replaced
// or removed by the next successful transaction.
type manifest struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
}

// journal is written before the first active-output move. Backup presence plus
// HadOld lets recovery infer every partially completed publication state without
// rewriting the journal after each file.
type journal struct {
	ID       string         `json:"id"`
	Staging  string         `json:"staging"`
	Rollback string         `json:"rollback"`
	Entries  []journalEntry `json:"entries"`
}

type journalEntry struct {
	Name   string `json:"name"`
	HadOld bool   `json:"hadOld"`
}

// New creates a transaction rooted at dir and recovers an interrupted prior
// publication before returning.
func New(dir string, driver Driver) (*Transaction, error) {
	if dir == "" {
		return nil, fmt.Errorf("output directory is empty")
	}
	if driver == nil {
		return nil, fmt.Errorf("output driver is nil")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve output directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	t := &Transaction{dir: abs, driver: driver, maxWriters: defaultMaxWriters}
	if err := t.Recover(); err != nil {
		return nil, err
	}
	return t, nil
}

// Register adds a normally scheduled artifact.
func (t *Transaction) Register(name string, value any) error {
	return t.register(name, value, false)
}

// RegisterExclusive adds an artifact that runs without another artifact writer.
// Use it for drivers that already parallelize internally or have a large memory peak.
func (t *Transaction) RegisterExclusive(name string, value any) error {
	return t.register(name, value, true)
}

// Len returns the number of registered artifacts.
func (t *Transaction) Len() int {
	return len(t.artifacts)
}

func (t *Transaction) register(name string, value any, exclusive bool) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	for _, existing := range t.artifacts {
		if existing.name == name {
			return fmt.Errorf("output %q is registered more than once", name)
		}
	}
	t.artifacts = append(t.artifacts, artifact{name: name, exclusive: exclusive, value: value})
	return nil
}

// Publish writes every artifact to staging and publishes the completed set. A
// staging failure leaves the active dataset untouched; a publication failure is
// rolled back before Publish returns.
func (t *Transaction) Publish() error {
	if err := t.Recover(); err != nil {
		return err
	}

	staging, err := os.MkdirTemp(t.dir, stagingPrefix)
	if err != nil {
		return fmt.Errorf("create output staging directory: %w", err)
	}
	removeStaging := true
	defer func() {
		if removeStaging {
			_ = os.RemoveAll(staging)
		}
	}()

	if err := t.writeArtifacts(staging); err != nil {
		return err
	}
	files := make([]string, len(t.artifacts))
	for i, artifact := range t.artifacts {
		files[i] = artifact.name
	}
	sort.Strings(files)
	if err := jsonio.WriteFile(filepath.Join(staging, manifestName), manifest{Version: 1, Files: files}, true); err != nil {
		return fmt.Errorf("write output ownership manifest: %w", err)
	}

	if err := t.publishStaged(staging, files); err != nil {
		recoveryErr := t.Recover()
		if recoveryErr != nil {
			return errors.Join(err, recoveryErr)
		}
		return err
	}
	removeStaging = false
	return nil
}

// Recover restores an uncommitted publication or finishes cleanup for a
// committed one. It is safe to call repeatedly.
func (t *Transaction) Recover() error {
	journalPath := filepath.Join(t.dir, journalName)
	b, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return t.cleanOrphans()
	}
	if err != nil {
		return fmt.Errorf("read output transaction journal: %w", err)
	}
	var tx journal
	if err := json.Unmarshal(b, &tx); err != nil {
		return fmt.Errorf("decode output transaction journal: %w", err)
	}
	if err := t.validateJournal(tx); err != nil {
		return err
	}

	// The commit marker is created only after every new file is in place. Its
	// presence changes recovery from restoring backups to discarding them.
	committed, err := os.ReadFile(filepath.Join(t.dir, committedName))
	if err == nil && string(committed) == tx.ID {
		return t.finishCommitted(tx)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read output transaction commit marker: %w", err)
	}
	return t.rollback(tx)
}

func (t *Transaction) writeArtifacts(staging string) error {
	writers := min(t.maxWriters, runtime.GOMAXPROCS(0), len(t.artifacts))
	if writers == 0 {
		writers = 1
	}

	// Normal artifacts are written in small concurrent batches. Exclusive
	// artifacts split those batches and run alone, avoiding nested parallel JSON
	// encoders and their combined allocation peak.
	start := 0
	for i, artifact := range t.artifacts {
		if !artifact.exclusive {
			continue
		}
		if err := t.writeBatch(staging, t.artifacts[start:i], writers); err != nil {
			return err
		}
		if err := t.writeBatch(staging, t.artifacts[i:i+1], 1); err != nil {
			return err
		}
		start = i + 1
	}
	return t.writeBatch(staging, t.artifacts[start:], writers)
}

func (t *Transaction) writeBatch(staging string, artifacts []artifact, workers int) error {
	if workers < 1 {
		return fmt.Errorf("output writer count must be positive")
	}
	if len(artifacts) == 0 {
		return nil
	}
	workers = min(workers, len(artifacts))

	jobs := make(chan artifact)
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	worker := func() {
		defer wg.Done()
		for artifact := range jobs {
			path := filepath.Join(staging, artifact.name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				errOnce.Do(func() { firstErr = fmt.Errorf("create output directory for %q: %w", artifact.name, err) })
				continue
			}
			if err := t.driver.Write(path, artifact.value); err != nil {
				errOnce.Do(func() { firstErr = fmt.Errorf("write output %q: %w", artifact.name, err) })
			}
		}
	}

	wg.Add(workers)
	for range workers {
		go worker()
	}
	for _, artifact := range artifacts {
		jobs <- artifact
	}
	close(jobs)
	wg.Wait()
	return firstErr
}

func (t *Transaction) publishStaged(staging string, nextFiles []string) error {
	if err := t.validateStaged(staging, nextFiles); err != nil {
		return err
	}
	previous, err := t.readManifest(filepath.Join(t.dir, manifestName))
	if err != nil {
		return err
	}

	// The union is the exact set this transaction may touch. A path absent from
	// both manifests belongs to the user or another pipeline and is never moved.
	owned := append(slices.Clone(previous.Files), nextFiles...)
	sort.Strings(owned)
	owned = slices.Compact(owned)
	owned = append(owned, manifestName) // publish ownership metadata last

	rollback, err := os.MkdirTemp(t.dir, rollbackPrefix)
	if err != nil {
		return fmt.Errorf("create output rollback directory: %w", err)
	}
	tx := journal{
		ID:       filepath.Base(staging),
		Staging:  filepath.Base(staging),
		Rollback: filepath.Base(rollback),
		Entries:  make([]journalEntry, 0, len(owned)),
	}
	for _, name := range owned {
		_, statErr := os.Lstat(filepath.Join(t.dir, name))
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect previous output %q: %w", name, statErr)
		}
		tx.Entries = append(tx.Entries, journalEntry{Name: name, HadOld: statErr == nil})
	}

	// The immutable journal reaches disk before the first active path moves.
	// Recovery derives progress from the target, staging, and rollback paths.
	if err := writeNewJSON(filepath.Join(t.dir, journalName), tx); err != nil {
		return fmt.Errorf("write output transaction journal: %w", err)
	}
	if err := t.callHook("journal"); err != nil {
		return err
	}

	for _, entry := range tx.Entries {
		target := filepath.Join(t.dir, entry.Name)
		backup := filepath.Join(rollback, entry.Name)
		staged := filepath.Join(staging, entry.Name)
		if entry.HadOld {
			if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				return fmt.Errorf("create rollback directory for %q: %w", entry.Name, err)
			}
			if err := os.Rename(target, backup); err != nil {
				return fmt.Errorf("move previous output %q to rollback: %w", entry.Name, err)
			}
			if err := t.callHook("old:" + entry.Name); err != nil {
				return err
			}
		}
		if _, err := os.Lstat(staged); err == nil {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create published output directory for %q: %w", entry.Name, err)
			}
			if err := os.Rename(staged, target); err != nil {
				return fmt.Errorf("publish output %q: %w", entry.Name, err)
			}
			if err := t.callHook("new:" + entry.Name); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect staged output %q: %w", entry.Name, err)
		}
	}

	if err := writeNewFile(filepath.Join(t.dir, committedName), []byte(tx.ID)); err != nil {
		return fmt.Errorf("mark output transaction committed: %w", err)
	}
	if err := t.callHook("committed"); err != nil {
		return err
	}
	return t.finishCommitted(tx)
}

func (t *Transaction) callHook(point string) error {
	if t.publishHook == nil {
		return nil
	}
	return t.publishHook(point)
}

func (t *Transaction) validateJournal(tx journal) error {
	if tx.ID == "" || tx.ID != tx.Staging || filepath.Base(tx.Staging) != tx.Staging || !strings.HasPrefix(tx.Staging, stagingPrefix) {
		return fmt.Errorf("invalid output transaction staging directory %q", tx.Staging)
	}
	if filepath.Base(tx.Rollback) != tx.Rollback || !strings.HasPrefix(tx.Rollback, rollbackPrefix) {
		return fmt.Errorf("invalid output transaction rollback directory %q", tx.Rollback)
	}
	seen := make(map[string]bool, len(tx.Entries))
	for _, entry := range tx.Entries {
		if entry.Name != manifestName {
			if _, err := cleanName(entry.Name); err != nil {
				return fmt.Errorf("invalid output transaction entry: %w", err)
			}
		}
		if seen[entry.Name] {
			return fmt.Errorf("duplicate output transaction entry %q", entry.Name)
		}
		seen[entry.Name] = true
	}
	return nil
}

func (t *Transaction) rollback(tx journal) error {
	rollback := filepath.Join(t.dir, tx.Rollback)
	for i := len(tx.Entries) - 1; i >= 0; i-- {
		entry := tx.Entries[i]
		target := filepath.Join(t.dir, entry.Name)
		backup := filepath.Join(rollback, entry.Name)

		// A backup proves the old file moved, so replace any installed new file
		// with it. Without a backup, a path that had no old file can only be a
		// partially installed addition and must be removed.
		if _, err := os.Lstat(backup); err == nil {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("remove interrupted output %q: %w", entry.Name, err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("recreate output directory for %q: %w", entry.Name, err)
			}
			if err := os.Rename(backup, target); err != nil {
				return fmt.Errorf("restore previous output %q: %w", entry.Name, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect rollback output %q: %w", entry.Name, err)
		} else if !entry.HadOld {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("remove new output %q during rollback: %w", entry.Name, err)
			}
		}
	}
	if err := os.RemoveAll(filepath.Join(t.dir, tx.Staging)); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(t.dir, tx.Rollback)); err != nil {
		return err
	}
	if err := removeIfExists(filepath.Join(t.dir, journalName)); err != nil {
		return err
	}
	return removeIfExists(filepath.Join(t.dir, committedName))
}

func (t *Transaction) finishCommitted(tx journal) error {
	// Keep the journal and commit marker until both scratch directories are gone.
	// A crash during cleanup therefore re-enters this committed branch safely.
	if err := os.RemoveAll(filepath.Join(t.dir, tx.Staging)); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(t.dir, tx.Rollback)); err != nil {
		return err
	}
	if err := removeIfExists(filepath.Join(t.dir, journalName)); err != nil {
		return err
	}
	return removeIfExists(filepath.Join(t.dir, committedName))
}

func (t *Transaction) readManifest(path string) (manifest, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return manifest{}, nil
	}
	if err != nil {
		return manifest{}, fmt.Errorf("read output ownership manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return manifest{}, fmt.Errorf("decode output ownership manifest: %w", err)
	}
	if m.Version != 1 {
		return manifest{}, fmt.Errorf("unsupported output ownership manifest version %d", m.Version)
	}
	seen := make(map[string]bool, len(m.Files))
	for _, name := range m.Files {
		if _, err := cleanName(name); err != nil {
			return manifest{}, fmt.Errorf("output ownership manifest: %w", err)
		}
		if seen[name] {
			return manifest{}, fmt.Errorf("output ownership manifest contains duplicate path %q", name)
		}
		seen[name] = true
	}
	return m, nil
}

func (t *Transaction) validateStaged(staging string, nextFiles []string) error {
	stagingAbs, err := filepath.Abs(staging)
	if err != nil {
		return err
	}
	if filepath.Dir(stagingAbs) != t.dir || !strings.HasPrefix(filepath.Base(stagingAbs), stagingPrefix) {
		return fmt.Errorf("output staging directory %q is not a direct child of %q", staging, t.dir)
	}

	files := slices.Clone(nextFiles)
	for _, name := range files {
		if _, err := cleanName(name); err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(staging, name))
		if err != nil {
			return fmt.Errorf("inspect staged output %q: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged output %q is not a regular file", name)
		}
	}
	sort.Strings(files)
	if len(files) != len(slices.Compact(slices.Clone(files))) {
		return fmt.Errorf("staged output list contains duplicate paths")
	}
	m, err := t.readManifest(filepath.Join(staging, manifestName))
	if err != nil {
		return fmt.Errorf("validate staged ownership manifest: %w", err)
	}
	manifestFiles := slices.Clone(m.Files)
	sort.Strings(manifestFiles)
	if !slices.Equal(files, manifestFiles) {
		return fmt.Errorf("staged ownership manifest does not match registered outputs")
	}
	return nil
}

func (t *Transaction) cleanOrphans() error {
	// No journal means none of these scratch paths can belong to a live commit.
	// They are remnants from a crash before the journal was atomically installed.
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && (strings.HasPrefix(name, stagingPrefix) || strings.HasPrefix(name, rollbackPrefix)) {
			if err := os.RemoveAll(filepath.Join(t.dir, name)); err != nil {
				return err
			}
		} else if !entry.IsDir() && strings.HasPrefix(name, markerPrefix) {
			if err := os.Remove(filepath.Join(t.dir, name)); err != nil {
				return err
			}
		}
	}
	return removeIfExists(filepath.Join(t.dir, committedName))
}

func cleanName(name string) (string, error) {
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("unsafe output path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe output path %q", name)
	}
	if clean != name {
		return "", fmt.Errorf("output path %q is not clean", name)
	}
	switch clean {
	case manifestName, journalName, committedName:
		return "", fmt.Errorf("output path %q is reserved", name)
	}
	return clean, nil
}

func writeNewJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeNewFile(path, append(b, '\n'))
}

func writeNewFile(path string, data []byte) error {
	// Markers are written beside their destination and renamed into place so a
	// process interruption exposes either no marker or the complete marker.
	temp, err := os.CreateTemp(filepath.Dir(path), markerPrefix)
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
