package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/internal/corpus"
	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/lint"
)

func TestConvertRoundTrip(t *testing.T) {
	for _, p := range testenv.Packs(t) {
		t.Run(filepath.Base(p), func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			js, resp, err := Convert(in, ConvertRequest{To: JSON})
			if err != nil || resp.From != KP || len(resp.Warnings) != 0 {
				t.Fatalf("kp to json: %v %+v", err, resp)
			}
			back, resp, err := Convert(js, ConvertRequest{To: JSON})
			if err != nil || resp.From != JSON || !bytes.Equal(back, js) {
				t.Fatalf("json to json: %v %+v", err, resp)
			}
			y, _, err := Convert(in, ConvertRequest{To: YAML})
			if err != nil {
				t.Fatalf("kp to yaml: %v", err)
			}
			back, resp, err = Convert(y, ConvertRequest{To: JSON})
			if err != nil || resp.From != YAML || len(resp.Warnings) != 0 || !bytes.Equal(back, js) {
				t.Fatalf("yaml to json: %v %+v", err, resp)
			}
			if v, err := Verify(y); err != nil || v.Status != "clean" {
				t.Fatalf("verify yaml: %v %+v", err, v)
			}
			if _, _, err := Convert(js, ConvertRequest{To: KP}); err != nil {
				t.Fatalf("json to kp: %v", err)
			}
			want, _, err := Convert(in, ConvertRequest{To: KP})
			if err != nil {
				t.Fatal(err)
			}
			out, resp, err := Convert(js, ConvertRequest{To: KP, Template: in})
			if err != nil || len(resp.Warnings) != 0 || !bytes.Equal(out, want) {
				t.Fatalf("json to kp with the original as template: %v %+v", err, resp)
			}
			x, resp, err := Convert(js, ConvertRequest{To: XDF})
			if err != nil || resp.Meta == nil {
				t.Fatalf("json to xdf: %v %+v", err, resp)
			}
			back, resp, err = Convert(x, ConvertRequest{To: JSON, Meta: resp.Meta})
			if err != nil || resp.From != XDF || len(resp.Warnings) != 0 || !bytes.Equal(back, js) {
				t.Fatalf("xdf with metadata to json: %v %+v", err, resp)
			}
		})
	}
}

func TestConvertEditedWarns(t *testing.T) {
	p := testenv.Packs(t)[0]
	in, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	js, _, err := Convert(in, ConvertRequest{To: JSON})
	if err != nil {
		t.Fatal(err)
	}
	edited := bytes.Replace(js, []byte(`"schema": `), []byte(`"schema":  `), 1)
	edited = bytes.Replace(edited, []byte(`"objects": [`), []byte(`"objects": [ `), 1)
	if _, resp, err := Convert(edited, ConvertRequest{To: JSON}); err != nil || len(resp.Warnings) != 0 {
		t.Errorf("whitespace only: %v %+v", err, resp)
	}
	i := bytes.Index(js, []byte(`"name": "`)) + len(`"name": "`)
	changed := append(append(append([]byte{}, js[:i]...), 'X'), js[i:]...)
	if _, resp, err := Convert(changed, ConvertRequest{To: JSON}); err != nil || len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "edited") {
		t.Errorf("value changed: %v %+v", err, resp)
	}
}

func TestCall(t *testing.T) {
	call := func(method, req string, in []byte) ([]byte, map[string]any) {
		t.Helper()
		out, resp := Call(method, []byte(req), in)
		var m map[string]any
		if err := json.Unmarshal(resp, &m); err != nil {
			t.Fatalf("%s: response %q: %v", method, resp, err)
		}
		return out, m
	}
	if _, m := call("version", "", nil); m["version"] != Version {
		t.Errorf("version: %v", m)
	}
	if _, m := call("nope", "", nil); !strings.Contains(m["error"].(string), "unknown method") {
		t.Errorf("unknown method: %v", m)
	}
	if _, m := call("convert", `{"to": "json", "extra": 1}`, nil); !strings.Contains(m["error"].(string), "request") {
		t.Errorf("unknown request field: %v", m)
	}
	if out, m := call("convert", `{"to": "json"}`, []byte("not a kp")); out != nil || m["error"] == nil {
		t.Errorf("bad input: %q %v", out, m)
	}
	p := testenv.Packs(t)[0]
	in, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	js, m := call("convert", `{"to": "json"}`, in)
	if m["from"] != "kp" || m["to"] != "json" || len(js) == 0 {
		t.Fatalf("convert: %v", m)
	}
	if _, m := call("verify", "", js); m["status"] != "clean" {
		t.Errorf("verify: %v", m)
	}
	x, m := call("convert", `{"to": "xdf"}`, js)
	meta, _ := m["meta"].(string)
	if meta == "" {
		t.Fatalf("convert to xdf: no metadata file: %v", m)
	}
	if back, m := call("convert", `{"to": "json", "meta": "`+meta+`"}`, x); m["from"] != "xdf" || !bytes.Equal(back, js) {
		t.Errorf("xdf with metadata to json: %v", m)
	}
	if _, m := call("convert", `{"to": "json", "meta": "`+meta+`"}`, js); m["error"] == nil {
		t.Errorf("metadata file with json input: %v", m)
	}
	v, m := call("convert", `{"to": "json", "name": "a.xdf"}`, x)
	var vm struct{ Provenance struct{ Format, File string } }
	if err := json.Unmarshal(v, &vm); err != nil || vm.Provenance.Format != "xdf" || vm.Provenance.File != "a.xdf" {
		t.Errorf("xdf alone to json: %v %v %+v", err, m, vm)
	}
}

// TestLintFix runs lint and fix through Call, with the image as base64, and
// checks that fixing the JSON dump and the KP file agree.
func TestLintFix(t *testing.T) {
	d := testenv.Archive(t)
	in, err := os.ReadFile(filepath.Join(d, "8D0907551G.kp"))
	if err != nil {
		testenv.Missing(t, "%v", err)
	}
	img := corpus.ArchiveImage(t, "8D0907551G")
	req, _ := json.Marshal(LintRequest{Image: img})
	_, resp := Call("lint", req, in)
	var lr LintResponse
	if err := json.Unmarshal(resp, &lr); err != nil || lr.Family != "me7" || len(lr.Findings) == 0 {
		t.Fatalf("lint: %v %s", err, resp[:min(len(resp), 200)])
	}
	out, resp := Call("fix", req, in)
	var fr FixResponse
	if err := json.Unmarshal(resp, &fr); err != nil || len(fr.Applied) == 0 || len(out) == 0 {
		t.Fatalf("fix: %v %s", err, resp[:min(len(resp), 200)])
	}
	for _, fd := range fr.Remaining {
		if fd.Confidence == "high" {
			t.Errorf("fix: %s remains", fd.ID)
		}
	}
	js, _, err := Convert(in, ConvertRequest{To: JSON})
	if err != nil {
		t.Fatal(err)
	}
	out2, _, err := Fix(js, FixRequest{Findings: lr.Findings[:0:0], To: KP})
	if err != nil || bytes.Equal(out2, out) {
		t.Fatalf("fix nothing from json: %v", err)
	}
	var high []lint.Finding
	for _, fd := range lr.Findings {
		if fd.Confidence == "high" {
			high = append(high, fd)
		}
	}
	out3, _, err := Fix(js, FixRequest{Findings: high})
	if err != nil {
		t.Fatal(err)
	}
	a, _, aerr := Convert(out, ConvertRequest{To: JSON})
	b, _, berr := Convert(out3, ConvertRequest{To: JSON})
	if errors.Join(aerr, berr) != nil || !bytes.Equal(stripStamp(t, a), stripStamp(t, b)) {
		t.Errorf("fix from json with given findings differs: %v %v", aerr, berr)
	}
}

// stripStamp drops the stamp and provenance, which differ between files with
// the same definitions.
func stripStamp(t *testing.T, js []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(js, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "stamp")
	delete(m, "provenance")
	b, _ := json.Marshal(m)
	return b
}
