package winx

import (
	"slices"
	"testing"
)

func TestProcessTree(t *testing.T) {
	parent := map[uint32]uint32{2: 1, 3: 2, 4: 3, 5: 9, 6: 1}
	got := processTree(1, parent)
	slices.Sort(got)
	if want := []uint32{1, 2, 3, 4, 6}; !slices.Equal(got, want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
}

func TestProcessTreeIgnoresPidReuseCycle(t *testing.T) {
	// A child whose recorded parent is itself must not loop forever.
	if got := processTree(1, map[uint32]uint32{7: 7, 2: 1}); len(got) != 2 {
		t.Fatalf("tree = %v", got)
	}
}

func TestTrimMemoryRuns(t *testing.T) {
	if TrimMemory() < 1 {
		t.Fatal("did not even trim this process")
	}
}
