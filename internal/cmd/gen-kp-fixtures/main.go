// Command gen-kp-fixtures writes testdata/kp/*-fewmaps.kp via kp.Shrink.
// V1/V2 use archive packs; 245/503/597 need testdata/local sources.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/kp"
)

func main() {
	root := testenv.ModuleRoot()
	out := filepath.Join(root, "testdata", "kp")
	cases := []struct {
		src, dst string
		n        int
	}{
		{"testdata/archive/ecuxplot/test8maps.kp", "v1-fewmaps.kp", 4},
		{"testdata/archive/ecuxplot/8D0907551K.kp", "v2-fewmaps.kp", 4},
		{"testdata/local/8N0906018AN-0004-245.kp", "245-fewmaps.kp", 4},
		{"testdata/local/4B0906018DH_0002.kp", "503-fewmaps.kp", 4},
		{"testdata/local/8N0906018BC_0002_366366.kp", "597-fewmaps.kp", 4},
	}
	for _, c := range cases {
		b, err := os.ReadFile(filepath.Join(root, c.src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", c.dst, err)
			continue
		}
		outb, err := kp.Shrink(b, c.n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", c.dst, err)
			os.Exit(1)
		}
		path := filepath.Join(out, c.dst)
		if err := os.WriteFile(path, outb, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		f, err := kp.Parse(outb)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reparse %s: %v\n", c.dst, err)
			os.Exit(1)
		}
		fmt.Printf("%s header=%d maps=%d size=%d\n", c.dst, f.HeaderID, len(f.Project.Maps), len(outb))
	}
}
