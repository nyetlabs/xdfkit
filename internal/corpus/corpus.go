// Package corpus gives tests access to ecu-corpus (docs/corpus.md): the
// corpus.tsv manifest, the images under images/, and categories.json. The
// corpus is found at XDFKIT_CORPUS, else ../ecu-corpus beside the module
// root, else ecu-corpus/ at the module root (the git submodule; see Dir).
// Without it, Open skips the test, or fails it when XDFKIT_REQUIRE_CORPUS is
// set (CI).
package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/internal/testenv"
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

// Dir is XDFKIT_CORPUS, else ../ecu-corpus beside the module root when it
// has a manifest, else ecu-corpus/ at the module root.
func Dir() string {
	if d := os.Getenv("XDFKIT_CORPUS"); d != "" {
		return d
	}
	root := testenv.ModuleRoot()
	if d := filepath.Join(filepath.Dir(root), "ecu-corpus"); fileExists(filepath.Join(d, "corpus.tsv")) {
		return d
	}
	return filepath.Join(root, "ecu-corpus")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Open returns the corpus, or skips (or fails) the test when it isn't there.
func Open(t testing.TB) *Corpus {
	t.Helper()
	dir := Dir()
	if _, err := os.Stat(filepath.Join(dir, "corpus.tsv")); err != nil {
		msg := fmt.Sprintf("corpus not available at %s (docs/corpus.md)", dir)
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

// Group is the images/ subdirectory for a manifest name: the first 9 characters
// of the part number (the stem up to '-'). A shorter part is its own directory.
func Group(name string) string {
	part, _, _ := strings.Cut(name, "-")
	if len(part) > 9 {
		return part[:9]
	}
	return part
}

// Rel is images/<group>/NAME.bin relative to the corpus root.
func Rel(name string) string {
	return filepath.Join("images", Group(name), name+".bin")
}

// Path returns the image file for a manifest name.
func (c *Corpus) Path(name string) string {
	return filepath.Join(c.Dir, Rel(name))
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
	for _, h := range []string{"name", "sha256", "size", "family", "ident", "oem", "checksums", "rsa"} {
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
			Checksums: f[col["checksums"]], RSA: f[col["rsa"]],
		})
	}
	return c, nil
}

// ArchiveImage returns the image for an archived ecuxplot pack stem: the
// archive's own STEM.bin if it has one (a non-OEM image the corpus can't hold),
// else the corpus image named in images.tsv, skipping the test without the
// corpus.
func ArchiveImage(t testing.TB, stem string) []byte {
	t.Helper()
	dir := testenv.Archive(t)
	if img, err := os.ReadFile(filepath.Join(dir, stem+".bin")); err == nil {
		return img
	}
	c := Open(t)
	b, err := os.ReadFile(filepath.Join(dir, "images.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if s, name, ok := strings.Cut(l, "\t"); ok && s == stem {
			img, err := os.ReadFile(c.Path(name))
			if err != nil {
				t.Fatal(err)
			}
			return img
		}
	}
	t.Fatalf("no image for %s in images.tsv", stem)
	return nil
}
