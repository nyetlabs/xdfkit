// Package testenv locates test inputs: the archived ecuxplot packs and jq.
// Missing inputs skip the test, or fail it when XDFKIT_REQUIRE_DATA is set (CI),
// so absent inputs can't turn a run silently green.
package testenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Missing skips the test, or fails it when XDFKIT_REQUIRE_DATA is set.
func Missing(t testing.TB, format string, args ...any) {
	t.Helper()
	if os.Getenv("XDFKIT_REQUIRE_DATA") != "" {
		t.Fatalf(format+" (XDFKIT_REQUIRE_DATA is set)", args...)
	}
	t.Skipf(format, args...)
}

// Archive returns testdata/archive/ecuxplot: the original ecuxplot KP packs
// and their mapdump CSVs, archived (see its README.md).
func Archive(t testing.TB) string {
	t.Helper()
	d := filepath.Join(ModuleRoot(), "testdata", "archive", "ecuxplot")
	if _, err := os.Stat(d); err != nil {
		Missing(t, "archive not found at %s", d)
	}
	return d
}

// Packs returns the archived KP files.
func Packs(t testing.TB) []string {
	t.Helper()
	d := Archive(t)
	kps, _ := filepath.Glob(filepath.Join(d, "*.kp"))
	if len(kps) == 0 {
		Missing(t, "no packs in %s", d)
	}
	return kps
}

// Jq checks that jq is installed.
func Jq(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		Missing(t, "jq not installed")
	}
}

// ModuleRoot is the nearest directory above the working directory with a go.mod.
func ModuleRoot() string {
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
