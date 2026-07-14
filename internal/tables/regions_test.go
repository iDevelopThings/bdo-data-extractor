package tables

import "testing"

func TestDecodeRegionsPreservesEmptyAndRepeatedRegions(t *testing.T) {
	data := []byte(`<RegionInfo Key="1"></RegionInfo>
<RegionInfo Key="2"><SpawnInfo key="10" dialogIndex="3" position="{1,2,3}"/></RegionInfo>
<RegionInfo Key="2"><SpawnInfo key="20" dialogIndex="4" position="{4,5,6}"/></RegionInfo>`)

	got, err := DecodeRegions(data)
	if err != nil {
		t.Fatalf("DecodeRegions: %v", err)
	}
	if spawns, exists := got[1]; !exists || spawns != nil {
		t.Fatalf("empty region = %v, exists %t, want nil and true", spawns, exists)
	}
	if len(got[2]) != 2 || got[2][0].Key != 10 || got[2][1].Key != 20 {
		t.Fatalf("repeated region spawns = %+v", got[2])
	}
}
