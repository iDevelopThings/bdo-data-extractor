package model

// MarketCategory is one Central Market main category with its sub-categories, in
// the game's display order (mains by category id, subs by sub id — matching item
// bytes @188/@189). Decoded from loc table 44.
type MarketCategory struct {
	ID            uint32              `json:"id"`
	Name          string              `json:"name"`
	SubCategories []MarketSubCategory `json:"subCategories,omitempty"`
}

// MarketSubCategory is a leaf category under a main category. Its ID matches item
// byte @189 (the sub-category id) within the parent main category.
type MarketSubCategory struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
}
