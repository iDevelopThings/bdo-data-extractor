package models_test

import (
	"strconv"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

// benchURNs registers a store of n entities against Default and returns their URNs.
func benchURNs(b *testing.B, n int) []urn.URN {
	b.Helper()
	models.ResetDefaultForTest()
	b.Cleanup(models.ResetDefaultForTest)

	s := models.NewStore[refTestEntity](n, nil)
	urns := make([]urn.URN, n)
	for i := range n {
		id := strconv.Itoa(i)
		urns[i] = refTestURN(id)
		if err := s.Add(urns[i], &refTestEntity{ID: id, Name: id}); err != nil {
			b.Fatal(err)
		}
	}
	models.RegisterStore(s)
	return urns
}

// Get(i) per element: one registry lookup (reflect + RLock + map) per element.
func BenchmarkEntityRefList_GetLoop_Uncached(b *testing.B) {
	urns := benchURNs(b, 20)
	b.ReportAllocs()
	for b.Loop() {
		l := models.NewEntityRefList[refTestEntity](urns...)
		for i := range l.Len() {
			_ = l.Get(i)
		}
	}
}

// The same work through the bulk path: one registry lookup for the whole list.
func BenchmarkEntityRefList_AllBulk_Uncached(b *testing.B) {
	urns := benchURNs(b, 20)
	b.ReportAllocs()
	for b.Loop() {
		l := models.NewEntityRefList[refTestEntity](urns...)
		_ = l.AllBulk()
	}
}

// The steady state a UI hits: the list resolved once, then read repeatedly.
func BenchmarkEntityRefList_AllBulk_Cached(b *testing.B) {
	urns := benchURNs(b, 20)
	l := models.NewEntityRefList[refTestEntity](urns...)
	_ = l.AllBulk()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = l.AllBulk()
	}
}

func BenchmarkEntityRefList_GetManyByIndex_Uncached(b *testing.B) {
	urns := benchURNs(b, 20)
	indices := make([]int, len(urns))
	for i := range indices {
		indices[i] = i
	}

	b.ReportAllocs()
	for b.Loop() {
		l := models.NewEntityRefList[refTestEntity](urns...)
		_ = l.GetManyByIndex(indices)
	}
}

func BenchmarkEntityRefList_GetManyByIndex_Cached(b *testing.B) {
	urns := benchURNs(b, 20)
	indices := make([]int, len(urns))
	for i := range indices {
		indices[i] = i
	}
	l := models.NewEntityRefList[refTestEntity](urns...)
	_ = l.AllBulk()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = l.GetManyByIndex(indices)
	}
}
