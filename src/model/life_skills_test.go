package model

import "testing"

func TestLifeSkillLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level      LifeSkillLevel
		grade      LifeSkillGrade
		gradeLevel int
		text       string
	}{
		{level: 0, grade: LifeSkillGradeUnknown, text: "Unknown 0"},
		{level: 1, grade: LifeSkillGradeBeginner, gradeLevel: 1, text: "Beginner 1"},
		{level: 10, grade: LifeSkillGradeBeginner, gradeLevel: 10, text: "Beginner 10"},
		{level: 11, grade: LifeSkillGradeApprentice, gradeLevel: 1, text: "Apprentice 1"},
		{level: 51, grade: LifeSkillGradeMaster, gradeLevel: 1, text: "Master 1"},
		{level: 80, grade: LifeSkillGradeMaster, gradeLevel: 30, text: "Master 30"},
		{level: 81, grade: LifeSkillGradeGuru, gradeLevel: 1, text: "Guru 1"},
		{level: 180, grade: LifeSkillGradeGuru, gradeLevel: 100, text: "Guru 100"},
		{level: 181, grade: LifeSkillGradeUnknown, text: "Unknown 181"},
	}

	for _, test := range tests {
		if got := test.level.Grade(); got != test.grade {
			t.Errorf("level %d grade = %v, want %v", test.level, got, test.grade)
		}
		if got := test.level.GradeLevel(); got != test.gradeLevel {
			t.Errorf("level %d grade level = %d, want %d", test.level, got, test.gradeLevel)
		}
		if got := test.level.String(); got != test.text {
			t.Errorf("level %d string = %q, want %q", test.level, got, test.text)
		}
	}
}

func TestLifeSkillTypeMetadata(t *testing.T) {
	t.Parallel()

	if LifeSkillTypes.Processing.Wire() != 5 || LifeSkillTypes.Processing.NativeName() != "processing" {
		t.Fatalf("processing metadata = %+v", LifeSkillTypes.Processing.Info())
	}
	if LifeSkillTypes.Processing.MasteryStat() != StatIdProcessingMastery {
		t.Fatalf("processing mastery stat = %v", LifeSkillTypes.Processing.MasteryStat())
	}
	if LifeSkillTypes.Quest.Wire() != 10 || LifeSkillTypes.Quest.Playable() {
		t.Fatalf("quest metadata = %+v", LifeSkillTypes.Quest.Info())
	}
	if LifeSkillTypes.Bartering.Wire() != 11 || !LifeSkillTypes.Bartering.Playable() {
		t.Fatalf("bartering metadata = %+v", LifeSkillTypes.Bartering.Info())
	}
}
