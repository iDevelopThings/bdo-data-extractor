package model

import "github.com/idevelopthings/bdo-data-extractor/src/urn"

// init binds each model type to its URN handler by Go type name. NewBaseFor[T]
// resolves the handler via urn.GetHandlerByType[T] (keyed by the Go type name),
// which the untyped domain registration in the urn package only satisfies for a
// couple of types by coincidence (Item, Recipe). Bind them all explicitly here,
// reusing the handler vars in src/urn as the single source of domain/kind truth.
func init() {
	urn.RegisterHandler[Item](urn.Item)
	urn.RegisterHandler[ItemSet](urn.ItemSet)
	urn.RegisterHandler[LightstoneCombination](urn.LightstoneCombination)
	urn.RegisterHandler[Enhancement](urn.Enhancement)
	urn.RegisterHandler[NPC](urn.NPC)
	urn.RegisterHandler[Character](urn.Character)
	urn.RegisterHandler[KnowledgeTheme](urn.Knowledge)
	urn.RegisterHandler[KnowledgeEntry](urn.Knowledge)
	urn.RegisterHandler[Zone](urn.GrindSpot)
	urn.RegisterHandler[WorldRegion](urn.World)
	urn.RegisterHandler[WorldNode](urn.World)
	urn.RegisterHandler[Territory](urn.World)
	urn.RegisterHandler[Recipe](urn.Recipe)
	urn.RegisterHandler[CaphrasCategory](urn.Caphras)
}
