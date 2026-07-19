package model

// ClassSkillData contains player skill rank chains and their per-class UI grids.
type ClassSkillData struct {
	Groups       []ClassSkillGroup        `json:"groups"`
	Trees        []ClassSkillTree         `json:"trees"`
	TreeMetadata []ClassSkillTreeMetadata `json:"treeMetadata"`
}

// ClassSkillTreeMetadata preserves subgroup string keys and its undecoded directory.
type ClassSkillTreeMetadata struct {
	Kind               string   `json:"kind"`
	SubGroupStringKeys []string `json:"subGroupStringKeys"`
	UnknownFooter      []byte   `json:"unknownFooter"`
}

// ClassSkillGroup is one learnable skill with its ordered ranks.
type ClassSkillGroup struct {
	Key     uint16               `json:"key"`
	Name    string               `json:"name,omitempty"`
	Classes []CharacterClassType `json:"classes"`
	Ranks   []ClassSkillRank     `json:"ranks"`
}

// ClassSkillRank is one selectable rank in a skill group.
type ClassSkillRank struct {
	Rank            int       `json:"rank"`
	SkillKey        uint32    `json:"skillKey"`
	SkillNo         uint16    `json:"skillNo"`
	SkillLevel      uint16    `json:"skillLevel"`
	Kind            SkillKind `json:"kind"`
	Name            string    `json:"name,omitempty"`
	Description     string    `json:"description,omitempty"`
	SourceName      string    `json:"sourceName,omitempty"`
	SourceGroupName string    `json:"sourceGroupName,omitempty"`
	Effects         *Effects  `json:"effects,omitempty"`
}

// ClassSkillTree is the combat or awakening skill grid for one class.
type ClassSkillTree struct {
	ClassType CharacterClassType   `json:"classType"`
	Kind      string               `json:"kind"`
	Width     int                  `json:"width"`
	Height    int                  `json:"height"`
	Cells     []ClassSkillTreeCell `json:"cells"`
}

// ClassSkillTreeCell preserves the position and drawing types of one grid cell.
type ClassSkillTreeCell struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Types     []byte `json:"types"`
	Group     uint16 `json:"group,omitempty"`
	SubGroup  byte   `json:"subGroup,omitempty"`
	Unknown16 byte   `json:"unknown16,omitempty"`
}
