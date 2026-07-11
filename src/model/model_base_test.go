package model

import (
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

func TestBaseForInitialize(t *testing.T) {

	item := &Item{
		BaseFor: models.NewBaseFor[Item](12345),
	}

	if item.GetURN().String() != "urn::item:12345" {
		t.Fatalf("unexpected URN: %s", item.GetURN().String())
	}
}
