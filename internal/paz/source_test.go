package paz

import "testing"

func TestSourceCloseReleasesActiveSource(t *testing.T) {
	s := &Source{Archive: Open(t.TempDir())}
	openMu.Lock()
	openSource = s
	openMu.Unlock()

	s.Close()

	openMu.Lock()
	active := openSource
	openMu.Unlock()
	if active != nil {
		t.Fatalf("closed source remains active: source=%p", active)
	}
}

func TestStaleSourceCloseKeepsActiveSource(t *testing.T) {
	stale := &Source{Archive: Open(t.TempDir())}
	active := &Source{Archive: Open(t.TempDir())}
	openMu.Lock()
	openSource = active
	openMu.Unlock()

	stale.Close()

	openMu.Lock()
	got := openSource
	openMu.Unlock()
	if got != active {
		t.Fatalf("stale close changed active source: source=%p", got)
	}
	active.Close()
}
