package kp

import (
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/xdfkit/internal/testenv"
)

func TestFlatDiffIdentical(t *testing.T) {
	p := filepath.Join(testenv.Archive(t), "8D0907551G.kp")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := Flat(f)
	if err != nil {
		t.Fatal(err)
	}
	if d := Diff(flat, flat, false); len(d) != 0 {
		t.Fatalf("identical: %d diffs, first %v", len(d), d[0])
	}
	if d := ByteDiff(b, b, 8); len(d) != 0 {
		t.Fatalf("byte identical: %v", d)
	}
}

func TestDiffNoise(t *testing.T) {
	a := map[string]string{
		"filename":                 `"A.KP"`,
		"headerLen":                "100",
		"project.trailing":         `"abcd"`,
		"project.maps[0].viewMode": "1",
	}
	b := map[string]string{
		"filename":                 `"B.KP"`,
		"headerLen":                "200",
		"project.trailing":         `"efgh"`,
		"project.maps[0].viewMode": "2",
	}
	d := Diff(a, b, false)
	if len(d) != 1 || d[0].Path != "project.maps[0].viewMode" {
		t.Fatalf("want viewMode only, got %#v", d)
	}
	d = Diff(a, b, true)
	if len(d) != 4 {
		t.Fatalf("keepNoise: want 4, got %#v", d)
	}
}

func TestByteDiff(t *testing.T) {
	a := []byte{0, 1, 2, 3}
	b := []byte{0, 9, 2, 3, 4}
	got := ByteDiff(a, b, 0)
	want := []string{"0x1: 01 -> 09", "length: 4 -> 5"}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v", got)
		}
	}
	if n := ByteDiff(a, b, 1); len(n) != 1 {
		t.Fatalf("limit: %#v", n)
	}
}
