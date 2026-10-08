// Package corpus gives tests access to the shared ecu-corpus (docs/corpus.md):
// the corpus.tsv manifest and the images under images/. The corpus is found at
// XDFKIT_CORPUS, else corpus/ at the module root (the git submodule). Without
// it, Open skips the test, or fails it when XDFKIT_REQUIRE_CORPUS is set (CI).
package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Image is one manifest row.
type Image struct {
	Name, SHA256, Family, Ident, OEM, Release, Checksums, RSA string
	Size                                                      int
}

// Corpus is an opened corpus.
type Corpus struct {
	Dir    string
	Images []Image
	bySHA  map[string]int
	byName map[string]int
}

// Open returns the corpus, or skips (or fails) the test when it isn't there.
func Open(t testing.TB) *Corpus {
	t.Helper()
	dir := os.Getenv("XDFKIT_CORPUS")
	if dir == "" {
		dir = filepath.Join(moduleRoot(), "corpus")
	}
	if _, err := os.Stat(filepath.Join(dir, "corpus.tsv")); err != nil {
		msg := fmt.Sprintf("corpus not available at %s (docs/corpus.md, Access)", dir)
		if os.Getenv("XDFKIT_REQUIRE_CORPUS") != "" {
			t.Fatal(msg + " (XDFKIT_REQUIRE_CORPUS is set)")
		}
		t.Skip(msg)
	}
	c, err := read(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Path returns the image file for a manifest name (images/NAME.bin).
func (c *Corpus) Path(name string) string {
	return filepath.Join(c.Dir, "images", name+".bin")
}

// ByName returns the image with that manifest name.
func (c *Corpus) ByName(name string) (Image, bool) {
	i, ok := c.byName[name]
	if !ok {
		return Image{}, false
	}
	return c.Images[i], true
}

// BySHA256 returns the image with that lowercase hex SHA-256.
func (c *Corpus) BySHA256(sum string) (Image, bool) {
	i, ok := c.bySHA[sum]
	if !ok {
		return Image{}, false
	}
	return c.Images[i], true
}

func read(dir string) (*Corpus, error) {
	path := filepath.Join(dir, "corpus.tsv")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	col := map[string]int{}
	for i, h := range strings.Split(lines[0], "\t") {
		col[h] = i
	}
	for _, h := range []string{"name", "sha256", "size", "family", "ident", "oem", "release", "checksums", "rsa"} {
		if _, ok := col[h]; !ok {
			return nil, fmt.Errorf("%s: no %q column", path, h)
		}
	}
	c := &Corpus{Dir: dir, bySHA: map[string]int{}, byName: map[string]int{}}
	for n, line := range lines[1:] {
		f := strings.Split(line, "\t")
		if len(f) != len(col) {
			return nil, fmt.Errorf("%s:%d: %d columns, want %d", path, n+2, len(f), len(col))
		}
		size, err := strconv.Atoi(f[col["size"]])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: size: %v", path, n+2, err)
		}
		c.byName[f[col["name"]]] = len(c.Images)
		c.bySHA[f[col["sha256"]]] = len(c.Images)
		c.Images = append(c.Images, Image{
			Name: f[col["name"]], SHA256: f[col["sha256"]], Size: size,
			Family: f[col["family"]], Ident: f[col["ident"]], OEM: f[col["oem"]],
			Release: f[col["release"]], Checksums: f[col["checksums"]], RSA: f[col["rsa"]],
		})
	}
	return c, nil
}

// moduleRoot is the nearest directory above the working directory with a go.mod.
func moduleRoot() string {
	dir, _ := os.Getwd()
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}
