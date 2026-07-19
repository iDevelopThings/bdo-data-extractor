package build

import (
	"fmt"
	"path/filepath"
)

// addJSON registers a normal JSON sidecar with the build's output transaction.
func (b *Builder) addJSON(name string, value any) (string, error) {
	return filepath.Base(name), b.outputs.Register(name, value)
}

// addExclusiveOutput registers a memory-heavy value that must be written alone.
func (b *Builder) addExclusiveOutput(name string, value any) error {
	return b.outputs.RegisterExclusive(name, value)
}

func (b *Builder) publishOutputs() error {
	if err := b.outputs.Publish(); err != nil {
		return err
	}
	b.logf(fmt.Sprintf("published %d output artifacts", b.outputs.Len()))
	return nil
}
