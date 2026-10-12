package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/canon"
)

// TestParse: one hyphen is for single-letter options, long options take two,
// and options may follow the files.
func TestParse(t *testing.T) {
	for _, s := range []struct {
		args, pos, refs []string
		format          string
		force           bool
		err             string
	}{
		{args: []string{"-force", "a"}, err: "--force"},
		{args: []string{"-format=xdf", "a"}, err: "--format"},
		{args: []string{"-version"}, err: "--version"},
		{args: []string{"-help"}, err: "--help"},
		{args: []string{"--force", "a", "b"}, pos: []string{"a", "b"}, force: true},
		{args: []string{"-fxdf", "a"}, pos: []string{"a"}, format: "xdf"},
		{args: []string{"a", "-f", "xdf", "b", "--force"}, pos: []string{"a", "b"}, format: "xdf", force: true},
		{args: []string{"--format", "kp", "-r", "x", "--ref", "y", "a"}, pos: []string{"a"}, format: "kp", refs: []string{"x", "y"}},
		{args: []string{"-", "--", "-force"}, pos: []string{"-", "-force"}},
	} {
		fs := newFlags("test")
		format := fs.StringP("format", "f", "", "")
		force := fs.Bool("force", false, "")
		refs := fs.StringArrayP("ref", "r", nil, "")
		pos, err := parse(fs, s.args)
		if s.err != "" {
			if err == nil || !strings.Contains(err.Error(), s.err) {
				t.Errorf("%q: error %v, want one naming %s", s.args, err, s.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", s.args, err)
			continue
		}
		if !slices.Equal(pos, s.pos) || *format != s.format || *force != s.force || !slices.Equal(*refs, s.refs) {
			t.Errorf("%q: pos %q format %q force %v refs %q, want %q %q %v %q", s.args, pos, *format, *force, *refs, s.pos, s.format, s.force, s.refs)
		}
	}
}

func TestVerifyStatus(t *testing.T) {
	js, err := canon.MarshalStamped(struct {
		N int `json:"n"`
	}{1}, "test")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.json")
	if err := os.WriteFile(clean, js, 0o644); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(dir, "edited.json")
	if err := os.WriteFile(edited, bytes.Replace(js, []byte(`"n": 1`), []byte(`"n": 2`), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := captureVerify(t, "--status", clean); got != "clean\n" {
		t.Errorf("clean: %q", got)
	}
	if got := captureVerify(t, "--status", edited); got != "edited\n" {
		t.Errorf("edited: %q", got)
	}
	if got := captureVerify(t, clean); !strings.HasPrefix(got, clean+": clean (") {
		t.Errorf("report: %q", got)
	}
}

func captureVerify(t *testing.T, args ...string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	err = verify(args)
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
