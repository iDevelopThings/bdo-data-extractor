package model

import "fmt"

// LifeSkillLevel is the client's raw life-skill level in the range 0-180.
type LifeSkillLevel uint16

// Grade resolves a raw level to Beginner through Guru.
func (l LifeSkillLevel) Grade() LifeSkillGrade {
	level := int(l)
	for _, info := range LifeSkillGrades.Infos() {
		if !info.Reserved && level >= info.MinLevel && level <= info.MaxLevel {
			return info.LifeSkillGrade
		}
	}
	return LifeSkillGradeUnknown
}

// GradeLevel returns the level within the resolved grade, or zero when invalid.
func (l LifeSkillLevel) GradeLevel() int {
	grade := l.Grade()
	if grade == LifeSkillGradeUnknown {
		return 0
	}
	return int(l) - grade.MinLevel() + 1
}

// String formats a raw level as its in-game grade and level.
func (l LifeSkillLevel) String() string {
	grade := l.Grade()
	if grade == LifeSkillGradeUnknown {
		return fmt.Sprintf("Unknown %d", l)
	}
	return fmt.Sprintf("%s %d", grade.Title(), l.GradeLevel())
}

// LifeSkillExperienceLevel is one lifeexp.dbss level requirement.
type LifeSkillExperienceLevel struct {
	Level              LifeSkillLevel `json:"level"`
	RequiredExperience uint64         `json:"requiredExperience"`
}

// LifeSkillProgression is one life-skill's complete level curve.
type LifeSkillProgression struct {
	Type     LifeSkillType              `json:"type"`
	MaxLevel LifeSkillLevel             `json:"maxLevel"`
	Levels   []LifeSkillExperienceLevel `json:"levels"`
}
