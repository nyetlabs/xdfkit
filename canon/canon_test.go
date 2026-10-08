package canon

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gowebpki/jcs"

	"go.nyet.org/xdfkit/kp"
)

type sample struct {
	Zeta  string            `json:"zeta"`
	Alpha []float64         `json:"alpha"`
	Mid   map[string]any    `json:"mid"`
	Empty map[string]string `json:"empty"`
	None  []int             `json:"none"`
	Null  *int              `json:"null"`
}

var edgy = sample{
	Zeta:  "a\x01\x7f\u2028<>&é€\t\"\\/ \b\f\r\n",
	Alpha: []float64{1, -30, 0.1, 0.0078125, 655.35, 0.3333333333333333, 1e-6, 1e20, 4294967295},
	Mid:   map[string]any{"b": true, "B": false, "ä": "x", "a": []any{map[string]any{}}},
	Empty: map[string]string{},
	None:  []int{},
}

var canonArgs = []string{"-S", "."}

// missing skips the test, or fails it when XDFKIT_REQUIRE_DATA is set (CI), so
// absent test inputs (ecuxplot data, jq) can't turn a run silently green.
func missing(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("XDFKIT_REQUIRE_DATA") != "" {
		t.Fatalf(format+" (XDFKIT_REQUIRE_DATA is set)", args...)
	}
	t.Skipf(format, args...)
}

func jq(t *testing.T, in []byte, args ...string) []byte {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		missing(t, "jq not installed")
	}
	cmd := exec.Command("jq", args...)
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jq %v: %v", args, err)
	}
	return out
}

func TestMatchesJq(t *testing.T) {
	got, err := Marshal(edgy)
	if err != nil {
		t.Fatal(err)
	}
	if want := jq(t, got, canonArgs...); !bytes.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestExponents(t *testing.T) {
	got, err := Marshal([]float64{4.7e-7, 5.96e-8, 1.23e-7, -7.31383e-307, 1e-6, 1.2e-6, 1e21, 1.5e300, -2.5e22})
	if err != nil {
		t.Fatal(err)
	}
	if want := jq(t, got, canonArgs...); !bytes.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestStamp(t *testing.T) {
	out, err := MarshalStamped(edgy, "xdfkit test")
	if err != nil {
		t.Fatal(err)
	}
	if want := jq(t, out, canonArgs...); !bytes.Equal(out, want) {
		t.Errorf("stamped output is not canonical")
	}
	stamped := func(canon string) string {
		return strings.TrimSpace(string(jq(t, out, "-r", "--arg", "c", canon,
			`.stamp.digests[] | select(.canon == $c) | .digest`)))
	}
	if d, want := stamped(CanonJq), fmt.Sprintf("sha256:%x", sha256.Sum256(jq(t, out, "-S", "del(.stamp)"))); d != want {
		t.Errorf("jq digest %s, sha256 of jq output %s", d, want)
	}
	j, err := jcs.Transform(jq(t, out, "-c", "del(.stamp)"))
	if err != nil {
		t.Fatal(err)
	}
	if d, want := stamped(CanonJCS), fmt.Sprintf("sha256:%x", sha256.Sum256(j)); d != want {
		t.Errorf("RFC 8785 digest %s, sha256 of JCS text %s", d, want)
	}
	check := func(what string, data []byte, want Status) {
		t.Helper()
		s, _, err := Verify(data)
		if err != nil || s != want {
			t.Errorf("%s: %v %v, want %v", what, s, err, want)
		}
	}
	check("as written", out, Clean)
	check("compact", jq(t, out, "-c", "."), Clean)
	check("value changed", bytes.Replace(out, []byte("655.35"), []byte("655.36"), 1), Edited)
	check("number respelled", bytes.Replace(out, []byte("-30.0"), []byte("-30"), 1), Mixed)
	check("no stamp", jq(t, out, "del(.stamp)"), Unstamped)
	check("unknown canon", jq(t, out, `.stamp.digests[].canon |= "x"`), Unknown)
}

// TestKPDumps checks the canonical dump of every ecuxplot pack against jq.
func TestKPDumps(t *testing.T) {
	dir := os.Getenv("XDFKIT_ECUXPLOT_DATA")
	if dir == "" {
		dir = filepath.Join("..", "..", "ecuxplot", "data")
	}
	kps, _ := filepath.Glob(filepath.Join(dir, "*.kp"))
	if len(kps) == 0 {
		missing(t, "no packs in %s (set XDFKIT_ECUXPLOT_DATA)", dir)
	}
	for _, p := range kps {
		t.Run(filepath.Base(p), func(t *testing.T) {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			f, err := kp.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			out, err := MarshalStamped(f, "xdfkit test")
			if err != nil {
				t.Fatal(err)
			}
			if want := jq(t, out, canonArgs...); !bytes.Equal(out, want) {
				t.Errorf("not canonical")
			}
			if s, _, err := Verify(out); err != nil || s != Clean {
				t.Errorf("verify: %v %v", s, err)
			}
			var back kp.File
			if err := Unmarshal(out, &back); err != nil {
				t.Fatal(err)
			}
			if again, err := MarshalStamped(&back, "xdfkit test"); err != nil || !bytes.Equal(again, out) {
				t.Errorf("decode and re-encode differs (%v)", err)
			}
		})
	}
}
