package a2l

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.nyet.org/xdfkit/internal/testenv"
)

func TestFragments(t *testing.T) {
	root := testenv.ModuleRoot()
	for _, name := range []string{
		"reciprocal.a2l", "damos-layout.a2l", "versionless.a2l",
	} {
		p := filepath.Join(root, "testdata", "a2l", name)
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
		m, warnings, err := Read(b, name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(m.Objects) == 0 {
			t.Errorf("%s: no objects", name)
		}
		if name == "versionless.a2l" && !slices.Contains(warnings, "ASAP2_VERSION missing") {
			t.Errorf("%s: warnings %v", name, warnings)
		}
		t.Logf("%s: %d objects, %d warnings", name, len(m.Objects), len(warnings))
	}
}

// TestFullSamples parses the large committed A2Ls when present.
func TestFullSamples(t *testing.T) {
	root := testenv.ModuleRoot()
	for _, name := range []string{
		"022906032CE-6733.A2L", "4B0906018CM-0008.A2L",
		"4B0906018DH-0002.A2L", "8N0906018BB-0002.A2L",
	} {
		p := filepath.Join(root, "testdata", "a2l", name)
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			testenv.Missing(t, "%s", p)
		}
		m, warnings, err := Read(b, name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(m.Objects) == 0 {
			t.Errorf("%s: no objects", name)
		}
		t.Logf("%s: %d objects, %d warnings", name, len(m.Objects), len(warnings))
	}
}
