package paz

import "testing"

func TestSourceCloseReleasesActiveSource(t *testing.T) {
	s := &Source{Archive: Open(t.TempDir())}
	openSource = s
	isOpen.Store(true)

	s.Close()

	if openSource != nil || isOpen.Load() {
		t.Fatalf("closed source remains active: source=%p open=%t", openSource, isOpen.Load())
	}
}

func TestStaleSourceCloseKeepsActiveSource(t *testing.T) {
	stale := &Source{Archive: Open(t.TempDir())}
	active := &Source{Archive: Open(t.TempDir())}
	openSource = active
	isOpen.Store(true)

	stale.Close()

	if openSource != active || !isOpen.Load() {
		t.Fatalf("stale close changed active source: source=%p open=%t", openSource, isOpen.Load())
	}
	active.Close()
}
