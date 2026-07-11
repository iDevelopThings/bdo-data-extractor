package models

import (
	"log"

	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

type Base struct {
	Urn urn.URN `json:"urn"`
}

type BaseFor[T any] struct {
	Base
}

func (b *BaseFor[T]) GetURN() urn.URN {
	return b.Urn
}

func NewBaseFor[T any](id uint32, kinds ...any) *BaseFor[T] {
	b := &BaseFor[T]{}
	return b.Initialize(id, kinds...)
}

func (b *BaseFor[T]) Initialize(id uint32, kinds ...any) *BaseFor[T] {
	h, ok := urn.GetHandlerByType[T]()
	if !ok {
		log.Fatalf("no urn handler registered for type %T", *new(T))
	}

	parts := []any{}
	if kinds != nil && len(kinds) > 0 {
		parts = append(parts, kinds...)
	}
	parts = append(parts, id)

	b.Urn = h.New(parts...)

	return b
}

// NewBaseForKey builds identity from arbitrary URN parts (in domain-then-id
// order, matching Handler.New), for entities whose id isn't a plain uint32 — a
// string slug (urn::character:<slug>) or a compound key like a recipe's
// (outputId, index).
func NewBaseForKey[T any](parts ...any) *BaseFor[T] {
	b := &BaseFor[T]{}
	return b.InitializeKey(parts...)
}

func (b *BaseFor[T]) InitializeKey(parts ...any) *BaseFor[T] {
	h, ok := urn.GetHandlerByType[T]()
	if !ok {
		log.Fatalf("no urn handler registered for type %T", *new(T))
	}

	b.Urn = h.New(parts...)

	return b
}
