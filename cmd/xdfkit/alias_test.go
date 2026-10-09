package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/api"
	"go.nyet.org/xdfkit/internal/testenv"
)

// TestAliasesMatchMakefile: make build links every alias.
func TestAliasesMatchMakefile(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(testenv.ModuleRoot(), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^ALIASES := (.*)$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("Makefile has no ALIASES line")
	}
	got := strings.Fields(string(m[1]))
	var want []string
	for n := range aliases {
		want = append(want, n)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("Makefile ALIASES %v, aliases %v", got, want)
	}
	for _, a := range []string{"/usr/local/bin/kp2xdf", `C:\xdfkit\kp2xdf.exe`, "kp2xdf"} {
		if n, ok := alias(a); !ok || n != "kp2xdf" {
			t.Errorf("alias(%q) = %q, %v", a, n, ok)
		}
	}
	if _, ok := alias("xdfkit"); ok {
		t.Error("xdfkit is an alias")
	}
}

// aliasInput copies the test KP pack to dir/p.kp and returns its contents.
func aliasInput(t *testing.T, dir string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(testenv.Archive(t), "test8maps.kp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.kp"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

// TestAliasConvert: each alias writes beside its input, never replaces a
// file, writes a metadata file only with -m, and refuses the wrong input
// format, unknown options and long options with one hyphen.
func TestAliasConvert(t *testing.T) {
	dir := t.TempDir()
	src := aliasInput(t, dir)
	kp := filepath.Join(dir, "p.kp")
	xdf := filepath.Join(dir, "p.xdf")
	for _, s := range []struct{ name, out string }{{"kp2xdf", xdf}, {"kp2json", filepath.Join(dir, "p.json")}} {
		if rc := runAlias(s.name, []string{kp}); rc != 0 {
			t.Fatalf("%s: rc %d", s.name, rc)
		}
		if _, err := os.Stat(s.out); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	if _, err := os.Stat(metaName(xdf)); err == nil {
		t.Error("kp2xdf without -m wrote a metadata file")
	}
	if rc := runAlias("xdf2kp", []string{xdf}); rc != 1 {
		t.Errorf("xdf2kp over an existing p.kp: rc %d, want 1", rc)
	}
	if b, _ := os.ReadFile(kp); !bytes.Equal(b, src) {
		t.Error("xdf2kp replaced p.kp")
	}
	for _, s := range []struct {
		name string
		args []string
		rc   int
	}{
		{"kp2xdf", []string{filepath.Join(dir, "p.json")}, 1},
		{"kp2json", nil, 2},
		{"kp2xdf", []string{"-force", kp}, 2},
		{"kp2xdf", []string{"--force", kp}, 2},
		{"kp2xdf", []string{"-version"}, 2},
		{"kp2xdf", []string{"-i"}, 2},
		{"kp2json", []string{"-m", kp}, 2},
		{"kp2xdf", []string{"-h"}, 0},
		{"kp2xdf", []string{"--help"}, 0},
		{"kp2xdf", []string{"--version"}, 0},
	} {
		if rc := runAlias(s.name, s.args); rc != s.rc {
			t.Errorf("%s %q: rc %d, want %d", s.name, s.args, rc, s.rc)
		}
	}
	q := filepath.Join(dir, "q.kp")
	img := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(q, src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(img, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if rc := runAlias("kp2xdf", []string{q, "-mi", img}); rc != 0 {
		t.Fatalf("kp2xdf q.kp -mi image: rc %d", rc)
	}
	if _, err := os.Stat(filepath.Join(dir, "q.meta.json")); err != nil {
		t.Errorf("kp2xdf -mi: %v", err)
	}
}

// modelOf is the model of a KP file, without its stamp and provenance.
func modelOf(t *testing.T, kp []byte) map[string]any {
	t.Helper()
	b, _, err := api.Convert(kp, api.ConvertRequest{To: api.JSON})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "stamp")
	delete(m, "provenance")
	return m
}

// TestAliasLinks runs the built binary through a symlink and as xdfkit
// kp2xdf: kp2xdf -m then xdf2kp gives back the definition, which needs the
// metadata file.
func TestAliasLinks(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("builds the binary and links it")
	}
	dir := t.TempDir()
	src := aliasInput(t, dir)
	bin := filepath.Join(dir, "xdfkit")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := os.Symlink("xdfkit", filepath.Join(dir, "xdf2kp")); err != nil {
		t.Fatal(err)
	}
	run := func(argv ...string) {
		t.Helper()
		cmd := exec.Command(filepath.Join(dir, argv[0]), argv[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}
	run("xdfkit", "kp2xdf", "-m", "p.kp")
	if err := os.Rename(filepath.Join(dir, "p.kp"), filepath.Join(dir, "orig.kp")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "p.xdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "q.xdf"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	run("xdf2kp", "p.xdf", "q.xdf")
	want := modelOf(t, src)
	for _, s := range []struct {
		name string
		same bool
	}{{"p.kp", true}, {"q.kp", false}} {
		b, err := os.ReadFile(filepath.Join(dir, s.name))
		if err != nil {
			t.Fatal(err)
		}
		if same := reflect.DeepEqual(modelOf(t, b), want); same != s.same {
			t.Errorf("%s: same definition as the input %v, want %v", s.name, same, s.same)
		}
	}
}
