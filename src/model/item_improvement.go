package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// ItemImprovements is the reform-chain sidecar from itemimprovement.dbss.
type ItemImprovements struct {
	Rows []ItemImprovement `json:"rows"`
}

// ItemImprovement is one gear reform: Result is crafted/reformed from Bases.
type ItemImprovement struct {
	Key    uint32                     `json:"key"`
	Result *models.EntityRef[Item]    `json:"result"`
	Bases  models.EntityRefList[Item] `json:"bases"`
	Flag   uint32                     `json:"flag"` // unidentified N×256 table value
}
