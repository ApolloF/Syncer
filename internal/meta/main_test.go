package meta

import (
	"os"
	"testing"

	"github.com/ApolloF/syncer/internal/discover"
)

// The tests don't depend on the game database this PC may have cached: a
// test that needs one sets its own.
func TestMain(m *testing.M) {
	discover.SetManifestForTest(nil)
	os.Exit(m.Run())
}
