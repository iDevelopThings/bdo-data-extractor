package models_test

import (
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

// refTestEntity is deliberately distinct from testItem (used in
// store_test.go / view_test.go) since EntityRef/EntityRefList resolve
// through the shared package-level Default registry, and a distinct
// type keeps this file's Default registrations from colliding with
// anything another test in this package might register.
type refTestEntity struct {
	ID   string
	Name string
}

func refTestURN(id string) urn.URN {
	return urn.URN{Domain: "reftest", ID: id}
}

// withDefaultStore registers a populated Store[refTestEntity] against
// Default for the duration of one test, and restores a clean Default
// afterward so tests don't leak state into each other.
func withDefaultStore(t *testing.T, entities map[string]string) {
	t.Helper()
	models.ResetDefaultForTest()
	t.Cleanup(models.ResetDefaultForTest)

	s := models.NewStore[refTestEntity](len(entities), nil)
	for id, name := range entities {
		if err := s.Add(refTestURN(id), &refTestEntity{ID: id, Name: name}); err != nil {
			t.Fatalf("Add(%s) returned error: %v", id, err)
		}
	}
	models.RegisterStore(s)
}

func TestEntityRef_GetValue_ResolvesFromDefaultRegistry(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword"})

	ref := models.NewEntityRef[refTestEntity](refTestURN("1"))

	v := ref.GetValue()
	if v == nil {
		t.Fatalf("GetValue() = nil, want resolved entity")
	}
	if v.Name != "Sword" {
		t.Errorf("GetValue().Name = %q, want %q", v.Name, "Sword")
	}
}

func TestEntityRef_GetValue_MemoizesAfterFirstResolve(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword"})

	ref := models.NewEntityRef[refTestEntity](refTestURN("1"))

	first := ref.GetValue()
	// Mutate the store's underlying map is not exposed, but we can prove
	// memoization by clearing Default entirely and confirming GetValue
	// still returns the already-cached value rather than re-resolving
	// (and failing) against an empty registry.
	models.ResetDefaultForTest()

	second := ref.GetValue()
	if second != first {
		t.Fatalf("GetValue() re-resolved after Default was reset; expected memoized value to be returned unchanged")
	}
}

func TestEntityRef_GetValue_UnresolvableURNReturnsNil(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword"})

	ref := models.NewEntityRef[refTestEntity](refTestURN("does-not-exist"))

	if v := ref.GetValue(); v != nil {
		t.Fatalf("GetValue() = %+v, want nil for an unresolvable URN", v)
	}
}

func TestEntityRef_GetValue_ZeroURNSkipsResolveEntirely(t *testing.T) {
	// No store registered at all — if GetValue tried to resolve a zero
	// URN it would still correctly get (nil, false) from Resolve, but
	// this pins down that the IsValid() short-circuit is actually taken
	// rather than relying on Resolve's own zero-URN handling.
	models.ResetDefaultForTest()
	t.Cleanup(models.ResetDefaultForTest)

	ref := &models.EntityRef[refTestEntity]{}
	if v := ref.GetValue(); v != nil {
		t.Fatalf("GetValue() on a zero-value ref = %+v, want nil", v)
	}
}

func TestEntityRef_SetValue_SetsBothFields(t *testing.T) {
	ref := &models.EntityRef[refTestEntity]{}
	entity := &refTestEntity{ID: "1", Name: "Sword"}

	ref.SetValue(entity, refTestURN("1"))

	if ref.Value != entity {
		t.Errorf("SetValue did not set Value")
	}
	if ref.URN != refTestURN("1") {
		t.Errorf("SetValue did not set URN, got %v", ref.URN)
	}
}

func TestEntityRef_SetURN_InvalidatesCachedValue(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword", "2": "Shield"})

	ref := models.NewEntityRef[refTestEntity](refTestURN("1"))
	if v := ref.GetValue(); v == nil || v.Name != "Sword" {
		t.Fatalf("initial GetValue() = %+v, want Sword", v)
	}

	ref.SetURN(refTestURN("2"))

	v := ref.GetValue()
	if v == nil {
		t.Fatalf("GetValue() after SetURN = nil, want resolved Shield")
	}
	if v.Name != "Shield" {
		t.Errorf("GetValue() after SetURN = %q, want %q (stale cached value not invalidated)", v.Name, "Shield")
	}
}

func TestEntityRefList_Get_ResolvesIndividualIndex(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword", "2": "Shield"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"), refTestURN("2"))

	got := list.Get(0)
	if got == nil || got.Name != "Sword" {
		t.Fatalf("Get(0) = %+v, want Sword", got)
	}
}

func TestEntityRefList_Get_OutOfRangeReturnsNil(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"))

	if got := list.Get(5); got != nil {
		t.Fatalf("Get(5) = %+v, want nil for out-of-range index", got)
	}
	if got := list.Get(-1); got != nil {
		t.Fatalf("Get(-1) = %+v, want nil for negative index", got)
	}
}

func TestEntityRefList_All_ResolvesEveryEntry(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword", "2": "Shield", "3": "Bow"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"), refTestURN("2"), refTestURN("3"))

	all := list.All()
	if len(all) != 3 {
		t.Fatalf("All() len = %d, want 3", len(all))
	}
	want := []string{"Sword", "Shield", "Bow"}
	for i, name := range want {
		if all[i] == nil || all[i].Name != name {
			t.Errorf("All()[%d] = %+v, want Name %q", i, all[i], name)
		}
	}
}

func TestEntityRefList_All_PartiallyUnresolvableEntriesAreNilNotError(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"), refTestURN("missing"))

	all := list.All()
	if len(all) != 2 {
		t.Fatalf("All() len = %d, want 2", len(all))
	}
	if all[0] == nil || all[0].Name != "Sword" {
		t.Errorf("All()[0] = %+v, want Sword", all[0])
	}
	if all[1] != nil {
		t.Errorf("All()[1] = %+v, want nil for an unresolvable URN", all[1])
	}
}

func TestEntityRefList_Get_MemoizesResolvedValue(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"))

	first := list.Get(0)
	models.ResetDefaultForTest() // if Get re-resolved, this would make it fail

	second := list.Get(0)
	if second != first {
		t.Fatalf("Get(0) re-resolved after Default was reset; expected memoized value")
	}
}

func TestEntityRefList_Add_AppendsURN(t *testing.T) {
	list := models.NewEntityRefList[refTestEntity](refTestURN("1"))
	list.Add(refTestURN("2"))

	if list.Len() != 2 {
		t.Fatalf("Len() = %d after Add, want 2", list.Len())
	}
	if list.URNs[1] != refTestURN("2") {
		t.Errorf("URNs[1] = %v, want %v", list.URNs[1], refTestURN("2"))
	}
}

// DO NOT DELETE
//
//type EntityRefData struct {
//	Fields []EntityRefField
//}
//
//var refsData = make(map[reflect.Type]EntityRefData)
//
//func IndexRefs[T any]() (reflect.Type, *EntityRefData) {
//
//	typ := reflect.TypeFor[T]()
//	if typ == nil {
//		log.Fatalf("failed to get type of %T", *new(T))
//	}
//	if typ.Kind() == reflect.Ptr {
//		typ = typ.Elem()
//	}
//
//	if data, ok := refsData[typ]; ok {
//		return typ, &data
//	}
//
//	isEntityRefType := func(typ reflect.Type) bool {
//		if typ.Kind() != reflect.Struct {
//			return false
//		}
//
//		n := typ.String()
//
//		// Output name of .String() is `models.EntityRef[other type full path]`
//		if strings.HasPrefix(n, "models.EntityRef[") {
//			return true
//		}
//
//		if strings.HasPrefix(n, "EntityRef[") {
//			return true
//		}
//
//		return false
//	}
//
//	data := EntityRefData{
//		Fields: []EntityRefField{},
//	}
//
//	for field := range typ.Fields() {
//		isEntityRef := false
//		isSlice := false
//
//		fKind := field.Type.Kind()
//		fElem := field.Type.Elem()
//		//n := fElem.String()
//
//		// We want to check if the field is a models.EntityRef
//		if fKind == reflect.Ptr && fElem.Kind() == reflect.Struct {
//			if isEntityRefType(fElem) {
//				isEntityRef = true
//				log.Printf("Field %s is an EntityRef", field.Name)
//
//				data.Fields = append(
//					data.Fields, EntityRefField{
//						Name:    field.Name,
//						Offset:  field.Offset,
//						Type:    fElem,
//						IsSlice: false,
//					},
//				)
//			}
//		} else if fKind == reflect.Slice && fElem.Kind() == reflect.Ptr {
//			if fElem.Elem().Name() == "EntityRef" {
//				isEntityRef = true
//				isSlice = true
//				log.Printf("Field %s is a slice of EntityRef", field.Name)
//			}
//		}
//
//		if !isEntityRef {
//			log.Printf("Field %s is not an EntityRef", field.Name)
//			continue
//		}
//
//		if isSlice {
//			log.Printf("Field %s is a slice of EntityRef, checking element type", field.Name)
//		}
//	}
//
//	refsData[typ] = data
//
//	return typ, &data
//}
//
//type EntityRefField struct {
//	Name    string
//	Offset  uintptr
//	Type    reflect.Type
//	IsSlice bool
//}
//
//func GetFieldData[T any, TEntity any](dataPtr *T, name string) *models.EntityRef[TEntity] {
//	_, refs := IndexRefs[T]()
//	if refs == nil {
//		log.Fatalf("failed to get EntityRef data for type %T", dataPtr)
//	}
//
//	// v := reflect.ValueOf(dataPtr).Elem()
//	vPtr := unsafe.Pointer(dataPtr)
//
//	var f *EntityRefField = nil
//	for _, ef := range refs.Fields {
//		if ef.Name == name {
//			f = &ef
//			break
//		}
//	}
//
//	if f == nil {
//		log.Fatalf("field %s not found in entityRefFields", name)
//	}
//
//	unsafeFieldPtr := unsafe.Add(vPtr, f.Offset)
//
//	return *(**models.EntityRef[TEntity])(unsafeFieldPtr)
//}
//
//func TestLoadingEntityRefs(t *testing.T) {
//
//	type TestStruct struct {
//		*models.BaseFor[TestStruct]
//
//		Item  *models.EntityRef[Item]   `json:"item"`
//		Items []*models.EntityRef[Item] `json:"items"`
//	}
//
//	urn.RegisterTypedHandler[TestStruct](
//		urn.WithKinds(),
//	)
//
//	te := &TestStruct{
//		BaseFor: models.NewBaseFor[TestStruct](1),
//		Item:    &models.EntityRef[Item]{URN: urn.Item.New(1)},
//		Items: []*models.EntityRef[Item]{
//			{URN: urn.Item.New(2)},
//			{URN: urn.Item.New(3)},
//		},
//	}
//	/*
//		typ := reflect.TypeOf(te).Elem()
//		if typ == nil {
//			t.Fatalf("failed to get type of TestStruct")
//		}
//
//		isEntityRefType := func(typ reflect.Type) bool {
//			if typ.Kind() != reflect.Struct {
//				return false
//			}
//
//			n := typ.String()
//
//			// Output name of .String() is `models.EntityRef[other type full path]`
//			if strings.HasPrefix(n, "models.EntityRef[") {
//				return true
//			}
//
//			if strings.HasPrefix(n, "EntityRef[") {
//				return true
//			}
//
//			return false
//		}
//
//		entityRefFields := []EntityRefField{}
//
//		for field := range typ.Fields() {
//			isEntityRef := false
//			isSlice := false
//
//			fKind := field.Type.Kind()
//			fElem := field.Type.Elem()
//			//n := fElem.String()
//
//			// We want to check if the field is a models.EntityRef
//			if fKind == reflect.Ptr && fElem.Kind() == reflect.Struct {
//				if isEntityRefType(fElem) {
//					isEntityRef = true
//					t.Logf("Field %s is an EntityRef", field.Name)
//
//					entityRefFields = append(
//						entityRefFields, EntityRefField{
//							Name:    field.Name,
//							Offset:  field.Offset,
//							Type:    fElem,
//							IsSlice: false,
//						},
//					)
//					t.Logf("Added EntityRefField: %+v", entityRefFields[len(entityRefFields)-1])
//
//				}
//			} else if fKind == reflect.Slice && fElem.Kind() == reflect.Ptr {
//				if fElem.Elem().Name() == "EntityRef" {
//					isEntityRef = true
//					isSlice = true
//					t.Logf("Field %s is a slice of EntityRef", field.Name)
//				}
//			}
//
//			if !isEntityRef {
//				t.Logf("Field %s is not an EntityRef", field.Name)
//				continue
//			}
//
//			if isSlice {
//				t.Logf("Field %s is a slice of EntityRef, checking element type", field.Name)
//			}
//		}
//
//		refField := getEntityRefFieldData(te, "Item")*/
//
//	refField := GetFieldData[TestStruct, Item](te, "Item")
//	if refField == nil {
//		t.Fatalf("failed to get EntityRef field data for Item")
//	}
//
//	// te.Item.GetValue() = *models.Item
//	// te.Item.Set(*models.Item)
//	// te.Item.Set(URN)
//
//}

func TestEntityRefList_GetManyByIndex_ReturnsSliceParallelToIndices(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword", "2": "Shield", "3": "Bow"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"), refTestURN("2"), refTestURN("3"))

	got := list.GetManyByIndex([]int{2})
	if len(got) != 1 {
		t.Fatalf("GetManyByIndex([2]) len = %d, want 1", len(got))
	}
	if got[0] == nil || got[0].Name != "Bow" {
		t.Fatalf("GetManyByIndex([2])[0] = %+v, want Bow", got[0])
	}
}

func TestEntityRefList_GetManyByIndex_OutOfRangeAndUnresolvableAreNil(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"), refTestURN("missing"))

	got := list.GetManyByIndex([]int{1, 0, 9, -1})
	if len(got) != 4 {
		t.Fatalf("GetManyByIndex len = %d, want 4", len(got))
	}
	if got[0] != nil {
		t.Errorf("got[0] = %+v, want nil for an unresolvable URN", got[0])
	}
	if got[1] == nil || got[1].Name != "Sword" {
		t.Errorf("got[1] = %+v, want Sword", got[1])
	}
	if got[2] != nil || got[3] != nil {
		t.Errorf("got[2]=%+v got[3]=%+v, want nil for out-of-range indices", got[2], got[3])
	}
}

// A URN that doesn't resolve must not shift the entries after it: the cache is
// positionally parallel to URNs, so a dropped entry would alias the wrong entity.
func TestEntityRefList_AllBulk_UnresolvableURNDoesNotShiftLaterEntries(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword", "3": "Bow"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"), refTestURN("missing"), refTestURN("3"))

	all := list.AllBulk()
	if len(all) != 3 {
		t.Fatalf("AllBulk() len = %d, want 3", len(all))
	}
	if all[0] == nil || all[0].Name != "Sword" {
		t.Errorf("AllBulk()[0] = %+v, want Sword", all[0])
	}
	if all[1] != nil {
		t.Errorf("AllBulk()[1] = %+v, want nil for an unresolvable URN", all[1])
	}
	if all[2] == nil || all[2].Name != "Bow" {
		t.Errorf("AllBulk()[2] = %+v, want Bow", all[2])
	}

	if got := list.Get(2); got == nil || got.Name != "Bow" {
		t.Errorf("Get(2) after AllBulk() = %+v, want Bow", got)
	}
}

// Get(i) sizes the Values cache to full length, so a later All() must not mistake
// that for "everything is resolved" and return a half-populated cache.
func TestEntityRefList_All_AfterPartialGetResolvesRemaining(t *testing.T) {
	withDefaultStore(t, map[string]string{"1": "Sword", "2": "Shield"})

	list := models.NewEntityRefList[refTestEntity](refTestURN("1"), refTestURN("2"))

	if got := list.Get(0); got == nil || got.Name != "Sword" {
		t.Fatalf("Get(0) = %+v, want Sword", got)
	}

	all := list.All()
	if len(all) != 2 {
		t.Fatalf("All() len = %d, want 2", len(all))
	}
	if all[1] == nil || all[1].Name != "Shield" {
		t.Errorf("All()[1] = %+v, want Shield (index 1 was never individually resolved)", all[1])
	}
}
