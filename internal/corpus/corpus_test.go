package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// TestImages checks every manifest row against its image, which also proves
// the corpus fetch in CI.
func TestImages(t *testing.T) {
	c := Open(t)
	if len(c.Images) == 0 {
		t.Fatal("empty manifest")
	}
	for _, im := range c.Images {
		data, err := os.ReadFile(c.Path(im.Name))
		if err != nil {
			t.Error(err)
			continue
		}
		sum := sha256.Sum256(data)
		if len(data) != im.Size || hex.EncodeToString(sum[:]) != im.SHA256 {
			t.Errorf("%s: size or SHA-256 differs from corpus.tsv", im.Name)
		}
		if got, ok := c.BySHA256(im.SHA256); !ok || got.Name != im.Name {
			t.Errorf("%s: BySHA256 lookup failed", im.Name)
		}
		if got, ok := c.ByName(im.Name); !ok || got.SHA256 != im.SHA256 {
			t.Errorf("%s: ByName lookup failed", im.Name)
		}
	}
}
