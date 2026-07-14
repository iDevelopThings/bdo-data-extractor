package model

import (
	"encoding/json"
	"testing"
)

func TestNPCSpawnTypesMarshalAsNumbers(t *testing.T) {
	npc := NPC{
		ID:   1,
		Name: "Manager",
		SpawnTypes: NPCSpawnTypes{
			NPCSpawnTypeImportantNPC,
			NPCSpawnTypeExplorer,
		},
	}

	data, err := json.Marshal(npc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(data), `{"id":1,"name":"Manager","spawnTypes":[4,12]}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestNPCSpawnTypesRoleQueries(t *testing.T) {
	normal := NPCSpawnTypes{NPCSpawnTypeNormal}
	if normal.HasMapRole() {
		t.Fatal("Normal unexpectedly counts as a map role")
	}

	npc := NPC{SpawnTypes: NPCSpawnTypes{NPCSpawnTypeNormal, NPCSpawnTypeExplorer}}
	if !npc.HasMapRole() {
		t.Fatal("Explorer did not count as a map role")
	}
	if !npc.HasSpawnType(NPCSpawnTypeExplorer) {
		t.Fatal("Explorer role was not found")
	}
	if npc.HasSpawnType(NPCSpawnTypeWarehouse) {
		t.Fatal("unset Warehouse role was found")
	}
}
