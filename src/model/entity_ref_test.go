package model

import (
	"log"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

type EntityRefData struct {
	Fields []EntityRefField
}

var refsData = make(map[reflect.Type]EntityRefData)

func IndexRefs[T any]() (reflect.Type, *EntityRefData) {

	typ := reflect.TypeFor[T]()
	if typ == nil {
		log.Fatalf("failed to get type of %T", *new(T))
	}
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if data, ok := refsData[typ]; ok {
		return typ, &data
	}

	isEntityRefType := func(typ reflect.Type) bool {
		if typ.Kind() != reflect.Struct {
			return false
		}

		n := typ.String()

		// Output name of .String() is `models.EntityRef[other type full path]`
		if strings.HasPrefix(n, "models.EntityRef[") {
			return true
		}

		if strings.HasPrefix(n, "EntityRef[") {
			return true
		}

		return false
	}

	data := EntityRefData{
		Fields: []EntityRefField{},
	}

	for field := range typ.Fields() {
		isEntityRef := false
		isSlice := false

		fKind := field.Type.Kind()
		fElem := field.Type.Elem()
		// n := fElem.String()

		// We want to check if the field is a models.EntityRef
		if fKind == reflect.Ptr && fElem.Kind() == reflect.Struct {
			if isEntityRefType(fElem) {
				isEntityRef = true
				log.Printf("Field %s is an EntityRef", field.Name)

				data.Fields = append(
					data.Fields, EntityRefField{
						Name:    field.Name,
						Offset:  field.Offset,
						Type:    fElem,
						IsSlice: false,
					},
				)
			}
		} else if fKind == reflect.Slice && fElem.Kind() == reflect.Ptr {
			if fElem.Elem().Name() == "EntityRef" {
				isEntityRef = true
				isSlice = true
				log.Printf("Field %s is a slice of EntityRef", field.Name)
			}
		}

		if !isEntityRef {
			log.Printf("Field %s is not an EntityRef", field.Name)
			continue
		}

		if isSlice {
			log.Printf("Field %s is a slice of EntityRef, checking element type", field.Name)
		}
	}

	refsData[typ] = data

	return typ, &data
}

type EntityRefField struct {
	Name    string
	Offset  uintptr
	Type    reflect.Type
	IsSlice bool
}

func GetFieldData[T any, TEntity any](dataPtr *T, name string) *models.EntityRef[TEntity] {
	_, refs := IndexRefs[T]()
	if refs == nil {
		log.Fatalf("failed to get EntityRef data for type %T", dataPtr)
	}

	// v := reflect.ValueOf(dataPtr).Elem()
	vPtr := unsafe.Pointer(dataPtr)

	var f *EntityRefField = nil
	for _, ef := range refs.Fields {
		if ef.Name == name {
			f = &ef
			break
		}
	}

	if f == nil {
		log.Fatalf("field %s not found in entityRefFields", name)
	}

	unsafeFieldPtr := unsafe.Add(vPtr, f.Offset)

	return *(**models.EntityRef[TEntity])(unsafeFieldPtr)
}

func TestLoadingEntityRefs(t *testing.T) {

	type TestStruct struct {
		*models.BaseFor[TestStruct]

		Item  *models.EntityRef[Item]   `json:"item"`
		Items []*models.EntityRef[Item] `json:"items"`
	}

	urn.RegisterTypedHandler[TestStruct](
		urn.WithKinds(),
	)

	te := &TestStruct{
		BaseFor: models.NewBaseFor[TestStruct](1),
		Item:    &models.EntityRef[Item]{URN: urn.Item.New(1)},
		Items: []*models.EntityRef[Item]{
			{URN: urn.Item.New(2)},
			{URN: urn.Item.New(3)},
		},
	}

	refField := GetFieldData[TestStruct, Item](te, "Item")
	if refField == nil {
		t.Fatalf("failed to get EntityRef field data for Item")
	}

	// te.Item.GetValue() = *models.Item
	// te.Item.Set(*models.Item)
	// te.Item.Set(URN)

}
