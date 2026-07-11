package models_test

import (
	"errors"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

type testItem struct {
	ID   string
	Name string
}

func itemURN(id string) urn.URN {
	return urn.URN{Domain: "testitem", ID: id}
}

func TestStore_AddAndGet(t *testing.T) {
	s := models.NewStore[testItem](0, nil)
	it := &testItem{ID: "1", Name: "Sword"}

	if err := s.Add(itemURN("1"), it); err != nil {
		t.Fatalf("Add returned unexpected error: %v", err)
	}

	got, ok := s.Get(itemURN("1"))
	if !ok {
		t.Fatalf("Get(1) ok = false, want true")
	}
	if got != it {
		t.Fatalf("Get(1) returned a different pointer than was added")
	}
}

func TestStore_Get_MissingReturnsFalse(t *testing.T) {
	s := models.NewStore[testItem](0, nil)

	if _, ok := s.Get(itemURN("nonexistent")); ok {
		t.Fatalf("Get on empty store returned ok = true, want false")
	}
}

func TestStore_Get_InvalidURNShortCircuits(t *testing.T) {
	s := models.NewStore[testItem](0, nil)
	// Deliberately do not add anything under the zero URN — Get should
	// reject it before ever probing the map, per IsValid().
	if _, ok := s.Get(urn.URN{}); ok {
		t.Fatalf("Get(zero URN) ok = true, want false")
	}
}

func TestStore_Add_ValidationRejectsWrongDomain(t *testing.T) {
	s := models.NewStore[testItem](
		0, func(u urn.URN) bool {
			return u.Domain == "testitem"
		},
	)

	err := s.Add(urn.URN{Domain: "wrongdomain", ID: "1"}, &testItem{ID: "1"})
	if err == nil {
		t.Fatalf("Add with wrong domain returned nil error, want an error")
	}
}

func TestStore_Add_ValidationAcceptsCorrectDomain(t *testing.T) {
	s := models.NewStore[testItem](
		0, func(u urn.URN) bool {
			return u.Domain == "testitem"
		},
	)

	if err := s.Add(itemURN("1"), &testItem{ID: "1"}); err != nil {
		t.Fatalf("Add with correct domain returned error: %v", err)
	}
}

func TestStore_All_PreservesInsertionOrder(t *testing.T) {
	s := models.NewStore[testItem](0, nil)

	ids := []string{"3", "1", "2"} // deliberately not sorted
	for _, id := range ids {
		if err := s.Add(itemURN(id), &testItem{ID: id}); err != nil {
			t.Fatalf("Add(%s) returned error: %v", id, err)
		}
	}

	all := s.All()
	if len(all) != len(ids) {
		t.Fatalf("All() len = %d, want %d", len(all), len(ids))
	}
	for i, id := range ids {
		if all[i].ID != id {
			t.Errorf("All()[%d].ID = %q, want %q (insertion order not preserved)", i, all[i].ID, id)
		}
	}
}

func TestStore_Add_DuplicateURNUpdatesInPlaceWithoutDuplicatingOrder(t *testing.T) {
	s := models.NewStore[testItem](0, nil)

	first := &testItem{ID: "1", Name: "Old Name"}
	second := &testItem{ID: "1", Name: "New Name"}

	if err := s.Add(itemURN("1"), first); err != nil {
		t.Fatalf("first Add returned error: %v", err)
	}
	if err := s.Add(itemURN("1"), second); err != nil {
		t.Fatalf("second Add returned error: %v", err)
	}

	if got := s.Len(); got != 1 {
		t.Fatalf("Len() = %d after re-adding same URN, want 1", got)
	}

	got, _ := s.Get(itemURN("1"))
	if got.Name != "New Name" {
		t.Fatalf("Get(1).Name = %q, want the second Add's value %q", got.Name, "New Name")
	}
}

func TestStore_Hooks_RunOncePerItemInRegistrationOrder(t *testing.T) {
	reg := models.NewRegistry()
	s := models.NewStore[testItem](0, nil)

	if err := s.Add(itemURN("1"), &testItem{ID: "1"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := s.Add(itemURN("2"), &testItem{ID: "2"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	var order []string
	s.AddHook(
		func(it *testItem) error {
			order = append(order, "hookA:"+it.ID)
			return nil
		},
	)
	s.AddHook(
		func(it *testItem) error {
			order = append(order, "hookB:"+it.ID)
			return nil
		},
	)

	models.RegisterStoreIn(reg, s)

	if err := reg.Build(); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	want := []string{"hookA:1", "hookB:1", "hookA:2", "hookB:2"}
	if len(order) != len(want) {
		t.Fatalf("hook call order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("hook call order[%d] = %q, want %q (full: %v)", i, order[i], want[i], order)
		}
	}

	// Build must not re-run hooks against already-processed items if
	// called again accidentally — that's a caller error, but Build
	// itself should still just replay hooks deterministically rather
	// than panicking or double-counting silently in some worse way.
	order = nil
	if err := reg.Build(); err != nil {
		t.Fatalf("second Build returned error: %v", err)
	}
	if len(order) != len(want) {
		t.Fatalf("second Build call order = %v, want %v", order, want)
	}
}

func TestStore_Hooks_ErrorShortCircuitsBuild(t *testing.T) {
	reg := models.NewRegistry()
	s := models.NewStore[testItem](0, nil)

	if err := s.Add(itemURN("1"), &testItem{ID: "1"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := s.Add(itemURN("2"), &testItem{ID: "2"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	wantErr := errors.New("boom")
	var secondItemProcessed bool

	s.AddHook(
		func(it *testItem) error {
			if it.ID == "1" {
				return wantErr
			}
			secondItemProcessed = true
			return nil
		},
	)

	models.RegisterStoreIn(reg, s)

	err := reg.Build()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Build() error = %v, want %v", err, wantErr)
	}
	if secondItemProcessed {
		t.Fatalf("hook ran for item 2 after item 1 failed; Build should short-circuit")
	}
}

func TestRegistry_CrossStoreResolveDuringBuild(t *testing.T) {
	// Simulates the Item/NPC cross-referencing case: two stores, a hook
	// on one that resolves against the other, both registered before
	// Build() runs — order of registration must not matter.
	type vendor struct {
		ID   string
		Name string
	}
	type product struct {
		ID             string
		VendorName     string
		ResolvedVendor *vendor
	}

	reg := models.NewRegistry()

	vendorURN := func(id string) urn.URN { return urn.URN{Domain: "vendor", ID: id} }
	productURN := func(id string) urn.URN { return urn.URN{Domain: "product", ID: id} }

	vendors := models.NewStore[vendor](0, nil)
	if err := vendors.Add(vendorURN("v1"), &vendor{ID: "v1", Name: "Alchemist"}); err != nil {
		t.Fatalf("Add vendor returned error: %v", err)
	}

	products := models.NewStore[product](0, nil)
	if err := products.Add(productURN("p1"), &product{ID: "p1", VendorName: "Alchemist"}); err != nil {
		t.Fatalf("Add product returned error: %v", err)
	}

	products.AddHook(
		func(p *product) error {
			v, ok := models.ResolveUrnIn[vendor](reg, vendorURN("v1"))
			if !ok {
				t.Fatalf("hook could not resolve vendor v1 — Build did not guarantee both stores were populated first")
			}
			p.ResolvedVendor = v
			return nil
		},
	)

	// Register products (with the hook that depends on vendors) before
	// vendors, to prove registration order doesn't create a dependency.
	models.RegisterStoreIn(reg, products)
	models.RegisterStoreIn(reg, vendors)

	if err := reg.Build(); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	p, _ := products.Get(productURN("p1"))
	if p.ResolvedVendor == nil {
		t.Fatalf("product p1 ResolvedVendor is nil after Build")
	}
	if p.ResolvedVendor.Name != "Alchemist" {
		t.Errorf("ResolvedVendor.Name = %q, want %q", p.ResolvedVendor.Name, "Alchemist")
	}
}

func TestRegistry_IsolatedFromDefaultAndOtherRegistries(t *testing.T) {
	regA := models.NewRegistry()
	regB := models.NewRegistry()

	sA := models.NewStore[testItem](0, nil)
	if err := sA.Add(itemURN("1"), &testItem{ID: "1", Name: "in A"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	models.RegisterStoreIn(regA, sA)

	if _, ok := models.ResolveUrnIn[testItem](regB, itemURN("1")); ok {
		t.Fatalf("regB resolved an item registered only against regA — registries are not isolated")
	}
	if _, ok := models.ResolveUrnIn[testItem](regA, itemURN("1")); !ok {
		t.Fatalf("regA failed to resolve its own registered item")
	}
}
