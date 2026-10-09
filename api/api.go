// Package api is the one surface every binding uses (docs/design.md, Bindings):
// the CLI, the planned rpc mode, the C ABI library and WebAssembly. Each method
// takes input bytes plus a request and returns output bytes plus a response.
// Go callers use the typed functions; the other surfaces go through Call, where
// requests and responses are canonical JSON. Nothing here touches files or the
// console.
package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"runtime/debug"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/kp"
	"go.nyet.org/xdfkit/lint"
	"go.nyet.org/xdfkit/mapcsv"
	"go.nyet.org/xdfkit/model"
	"go.nyet.org/xdfkit/xdf"
)

// Version is set from git describe by the Makefile. Do not edit it here.
var Version = "dev"

// Formats. CSV is output only. YAML is the model JSON in YAML syntax.
const (
	JSON = "json"
	YAML = "yaml"
	KP   = "kp"
	CSV  = "csv"
	XDF  = "xdf"
)

// ConvertRequest selects the output format; the input format is detected.
// Name is the input's file name, recorded in the model's provenance. Template
// is a KP file for KP output from model JSON: KP fields the model doesn't
// carry come from it (docs/model.md). Image is the flash image, for CSV value
// columns and XDF (file size, "subtract" axis labels). Refs, for CSV output,
// are reference definitions (KP or model JSON) whose matching map names fill
// one column each. Title is the XDF definition title (default: Name without
// its extension). Meta is the metadata file of an XDF input
// (docs/stamp-and-metadata.md). Origin, for KP or XDF input, is where its
// definitions came from (model.Origins), recorded in the provenance; it
// defaults to hand, or for an XDF to its metadata file's.
type ConvertRequest struct {
	To       string `json:"to"`
	Name     string `json:"name,omitempty"`
	Template []byte `json:"template,omitempty"`
	Image    []byte `json:"image,omitempty"`
	Refs     []Ref  `json:"refs,omitempty"`
	Title    string `json:"title,omitempty"`
	Meta     []byte `json:"meta,omitempty"`
	Origin   string `json:"origin,omitempty"`
}

// Ref is a reference definition for CSV output; Name heads its column.
type Ref struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}

// ConvertResponse reports what was converted and any warnings. Meta is the
// metadata file that goes with XDF output.
type ConvertResponse struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Warnings []string `json:"warnings,omitempty"`
	Meta     []byte   `json:"meta,omitempty"`
}

// Convert converts a KP file, a model JSON or YAML document (docs/model.md)
// or an XDF, with its metadata file if given, into To (json, yaml, kp, csv or
// xdf). JSON and YAML output is the stamped canonical model; Name, the input's
// file name, goes into its provenance. KP input to KP output is re-encoded as
// is. XDF output comes with its metadata file. Loading a hand-edited document,
// an XDF edited since its metadata file was written, or a template other than
// the file the model was read from, adds a warning.
func Convert(in []byte, req ConvertRequest) ([]byte, ConvertResponse, error) {
	resp := ConvertResponse{From: Detect(in), To: req.To}
	if req.Meta != nil && resp.From != XDF {
		return nil, resp, errors.New("a metadata file applies only to XDF input")
	}
	if req.Template != nil && (req.To != KP || resp.From == KP) {
		return nil, resp, errors.New("a template applies only to KP output from a model document")
	}
	if req.Image != nil && req.To != CSV && req.To != XDF {
		return nil, resp, errors.New("an image applies only to CSV and XDF output")
	}
	if req.Refs != nil && req.To != CSV {
		return nil, resp, errors.New("refs apply only to CSV output")
	}
	if req.Title != "" && req.To != XDF {
		return nil, resp, errors.New("a title applies only to XDF output")
	}
	if req.Origin != "" && resp.From != XDF && (resp.From != KP || req.To == KP) {
		return nil, resp, errors.New("an origin applies only to a model converted from KP or XDF")
	}
	if resp.From == KP && req.To == KP {
		f, err := kp.Parse(in)
		if err != nil {
			return nil, resp, err
		}
		out, err := f.Encode()
		return out, resp, err
	}
	m, warnings, err := loadModel(in, req.Name, req.Meta)
	resp.Warnings = warnings
	if err != nil {
		return nil, resp, err
	}
	if req.Origin != "" {
		m.Provenance.Origin = req.Origin
		if err := m.Check(); err != nil {
			return nil, resp, err
		}
	}
	if req.To == CSV {
		refs := make([]mapcsv.Ref, len(req.Refs))
		for i, r := range req.Refs {
			rm, w, err := loadModel(r.Data, r.Name, nil)
			for _, s := range w {
				resp.Warnings = append(resp.Warnings, r.Name+": "+s)
			}
			if err != nil {
				return nil, resp, fmt.Errorf("%s: %w", r.Name, err)
			}
			refs[i] = mapcsv.Ref{Name: r.Name, Model: rm}
		}
		out, err := mapcsv.Write(m, req.Image, refs)
		return out, resp, err
	}
	if req.To == XDF {
		title := req.Title
		if title == "" {
			title = strings.TrimSuffix(req.Name, path.Ext(req.Name))
		}
		out, err := xdf.Write(m, req.Image, title)
		if err != nil {
			return nil, resp, err
		}
		resp.Meta, err = xdf.Meta(m, out, "xdfkit "+Version)
		return out, resp, err
	}
	var t *kp.File
	if req.Template != nil {
		if t, err = kp.Parse(req.Template); err != nil {
			return nil, resp, fmt.Errorf("template: %w", err)
		}
		sum := sha256.Sum256(req.Template)
		if m.Provenance == nil || m.Provenance.SHA256 != hex.EncodeToString(sum[:]) {
			resp.Warnings = append(resp.Warnings, "template is not the file the model was read from")
		}
	}
	out, err := encodeModel(m, req.To, t)
	return out, resp, err
}

// loadModel reads a KP file, a model JSON or YAML document, or an XDF with
// its metadata file (nil: none). A hand-edited document adds a warning.
func loadModel(in []byte, name string, meta []byte) (*model.Model, []string, error) {
	switch Detect(in) {
	case KP:
		m, err := model.ReadKP(in, name)
		return m, nil, err
	case XDF:
		m, warnings, err := xdf.Read(in, meta)
		if err != nil {
			return m, warnings, err
		}
		if meta == nil || m.Provenance == nil {
			sum := sha256.Sum256(in)
			m.Provenance = &model.Provenance{Format: XDF, File: name, SHA256: hex.EncodeToString(sum[:])}
		}
		if m.Provenance.Origin == "" {
			m.Provenance.Origin = "hand"
		}
		return m, warnings, nil
	}
	in, err := asJSON(in)
	if err != nil {
		return nil, nil, err
	}
	v, err := Verify(in)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	if v.Status != "clean" && v.Status != "unstamped" {
		warnings = append(warnings, "stamp "+v.String())
	}
	m := new(model.Model)
	if err := canon.Unmarshal(in, m); err != nil {
		return nil, warnings, err
	}
	return m, warnings, m.Check()
}

// asJSON returns a model document as JSON, converting YAML.
func asJSON(in []byte) ([]byte, error) {
	if Detect(in) == YAML {
		return canon.YAMLToJSON(in)
	}
	return in, nil
}

// load reads a KP file or a model JSON or YAML document as a KP file.
func load(in []byte) (*kp.File, []string, error) {
	if Detect(in) == KP {
		f, err := kp.Parse(in)
		return f, nil, err
	}
	m, warnings, err := loadModel(in, "", nil)
	if err != nil {
		return nil, warnings, err
	}
	f, err := m.KP(nil)
	return f, warnings, err
}

// encodeModel writes m as To; KP output takes the fields the model doesn't
// carry from template (nil: defaults).
func encodeModel(m *model.Model, to string, template *kp.File) ([]byte, error) {
	switch to {
	case JSON, YAML:
		b, err := canon.MarshalStamped(m, "xdfkit "+Version)
		if err != nil || to == JSON {
			return b, err
		}
		return canon.JSONToYAML(b)
	case KP:
		f, err := m.KP(template)
		if err != nil {
			return nil, err
		}
		return f.Encode()
	}
	return nil, fmt.Errorf("unknown output format %q", to)
}

func encode(f *kp.File, to string) ([]byte, error) {
	if to == KP {
		return f.Encode()
	}
	return encodeModel(model.FromKP(f), to, nil)
}

// kpMagic starts every KP file: the length-prefixed "WinOLS File" signature.
var kpMagic = []byte("\x0b\x00\x00\x00WinOLS File")

// Detect names the input format from its contents: json for a JSON object,
// xdf for XML, kp for the KP signature, else yaml.
func Detect(in []byte) string {
	text := bytes.TrimLeft(bytes.TrimPrefix(in, []byte("\uFEFF")), " \t\r\n")
	switch {
	case bytes.HasPrefix(text, []byte("{")):
		return JSON
	case bytes.HasPrefix(text, []byte("<")):
		return XDF
	case bytes.HasPrefix(in, kpMagic):
		return KP
	}
	return YAML
}

// VerifyResponse is the stamp status of a JSON document: clean, edited, mixed,
// unknown or unstamped overall, and per digest.
type VerifyResponse struct {
	Status  string        `json:"status"`
	Digests []DigestCheck `json:"digests,omitempty"`
}

// DigestCheck is the result for one digest in the stamp.
type DigestCheck struct {
	Canon  string `json:"canon"`
	Digest string `json:"digest"`
	Status string `json:"status"`
}

// String renders the overall status with the per-digest results.
func (v VerifyResponse) String() string {
	if len(v.Digests) == 0 {
		return v.Status
	}
	parts := make([]string, len(v.Digests))
	for i, d := range v.Digests {
		parts[i] = d.Canon + " " + d.Status
	}
	return fmt.Sprintf("%s (%s)", v.Status, strings.Join(parts, ", "))
}

// Verify checks each digest in a JSON or YAML document's stamp.
func Verify(in []byte) (VerifyResponse, error) {
	if Detect(in) == XDF {
		return VerifyResponse{}, errors.New("XDF stamps can't be verified yet")
	}
	in, err := asJSON(in)
	if err != nil {
		return VerifyResponse{}, err
	}
	s, checks, err := canon.Verify(in)
	if err != nil {
		return VerifyResponse{}, err
	}
	v := VerifyResponse{Status: s.String()}
	for _, c := range checks {
		v.Digests = append(v.Digests, DigestCheck{c.Canon, c.Digest.Digest, c.Status.String()})
	}
	return v, nil
}

// LintRequest carries the flash image the KP file describes, and optionally
// the ECU family (me7 or m3) when the image has no Bosch identification.
type LintRequest struct {
	Image  []byte `json:"image"`
	Family string `json:"family,omitempty"`
}

// LintResponse lists the findings (docs/autocorrect.md).
type LintResponse struct {
	Family       string         `json:"family"`
	FamilySource string         `json:"familySource"`
	Findings     []lint.Finding `json:"findings"`
	Warnings     []string       `json:"warnings,omitempty"`
}

// Lint checks a KP file (or its model JSON) against its image.
func Lint(in []byte, req LintRequest) (LintResponse, error) {
	f, warnings, err := load(in)
	resp := LintResponse{Warnings: warnings}
	if err != nil {
		return resp, err
	}
	err = resp.lint(f, req)
	return resp, err
}

func (r *LintResponse) lint(f *kp.File, req LintRequest) error {
	if len(req.Image) == 0 {
		return errors.New("no image")
	}
	rep, err := lint.Lint(f, req.Image, lint.Options{Family: req.Family})
	r.Family, r.FamilySource, r.Findings = rep.Family, rep.FamilySource, rep.Findings
	return err
}

// FixRequest selects the fixes to make. Findings, if given (for example an
// edited lint response), are applied as they are; otherwise the file is
// linted against Image and the findings are chosen by Only (finding IDs), or
// by Rules and MinConfidence (default high). To is the output format
// (default kp).
type FixRequest struct {
	LintRequest
	Rules         []string       `json:"rules,omitempty"`
	Only          []string       `json:"only,omitempty"`
	MinConfidence string         `json:"minConfidence,omitempty"`
	Findings      []lint.Finding `json:"findings,omitempty"`
	To            string         `json:"to,omitempty"`
}

// FixResponse lists the IDs of the applied findings and, when the image was
// given, the findings that remain after the fix.
type FixResponse struct {
	Applied   []string       `json:"applied"`
	Remaining []lint.Finding `json:"remaining,omitempty"`
	Warnings  []string       `json:"warnings,omitempty"`
}

// Fix applies lint fixes to a KP file and re-encodes it. The output is
// parsed back and must match the fixed file.
func Fix(in []byte, req FixRequest) ([]byte, FixResponse, error) {
	f, warnings, err := load(in)
	resp := FixResponse{Applied: []string{}, Warnings: warnings}
	if err != nil {
		return nil, resp, err
	}
	sel := req.Findings
	if sel == nil {
		var lr LintResponse
		if err := lr.lint(f, req.LintRequest); err != nil {
			return nil, resp, err
		}
		min := req.MinConfidence
		if min == "" {
			min = lint.High
		}
		sel = lint.Select(lr.Findings, req.Rules, req.Only, min)
	}
	if err := lint.Apply(f, sel); err != nil {
		return nil, resp, err
	}
	for _, fd := range sel {
		resp.Applied = append(resp.Applied, fd.ID)
	}
	if req.To == "" {
		req.To = KP
	}
	out, err := encode(f, req.To)
	if err != nil {
		return nil, resp, err
	}
	g, _, err := load(out)
	if err != nil {
		return nil, resp, fmt.Errorf("fixed file doesn't parse: %w", err)
	}
	var want, got any = f, g
	if req.To != KP {
		want, got = model.FromKP(f), model.FromKP(g)
	}
	a, aerr := json.Marshal(want)
	b, berr := json.Marshal(got)
	if err := errors.Join(aerr, berr); err != nil {
		return nil, resp, err
	}
	if !bytes.Equal(a, b) {
		return nil, resp, errors.New("fixed file doesn't parse back to the fixed definitions")
	}
	if len(req.Image) > 0 {
		var lr LintResponse
		if err := lr.lint(g, req.LintRequest); err != nil {
			return nil, resp, err
		}
		resp.Remaining = lr.Findings
	}
	return out, resp, nil
}

// VersionResponse is the xdfkit version.
type VersionResponse struct {
	Version string `json:"version"`
}

// errorResponse is Call's response when the method fails.
type errorResponse struct {
	Error string `json:"error"`
}

// Call runs a method by name, with the request as JSON (empty means {}), and
// returns the output bytes and the response as canonical JSON. A failure,
// including a panic, returns no output and {"error": "..."}. Methods: convert
// (ConvertRequest), lint (LintRequest), fix (FixRequest), verify and version
// (no request fields). Images, templates and metadata files in requests and
// responses are base64.
func Call(method string, request, input []byte) (out, response []byte) {
	defer func() {
		if r := recover(); r != nil {
			out, response = nil, failure(fmt.Errorf("internal error: %v\n%s", r, debug.Stack()))
		}
	}()
	var resp any
	var err error
	switch method {
	case "convert":
		var req ConvertRequest
		if err = decode(request, &req); err == nil {
			out, resp, err = Convert(input, req)
		}
	case "verify":
		if err = decode(request, &struct{}{}); err == nil {
			resp, err = Verify(input)
		}
	case "lint":
		var req LintRequest
		if err = decode(request, &req); err == nil {
			resp, err = Lint(input, req)
		}
	case "fix":
		var req FixRequest
		if err = decode(request, &req); err == nil {
			out, resp, err = Fix(input, req)
		}
	case "version":
		if err = decode(request, &struct{}{}); err == nil {
			resp = VersionResponse{Version}
		}
	default:
		err = fmt.Errorf("unknown method %q", method)
	}
	if err != nil {
		return nil, failure(err)
	}
	b, err := canon.Marshal(resp)
	if err != nil {
		return nil, failure(err)
	}
	return out, b
}

func decode(request []byte, v any) error {
	if len(bytes.TrimSpace(request)) == 0 {
		return nil
	}
	if err := canon.Unmarshal(request, v); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	return nil
}

func failure(err error) []byte {
	b, merr := canon.Marshal(errorResponse{err.Error()})
	if merr != nil {
		b, _ = json.Marshal(errorResponse{errors.Join(err, merr).Error()})
	}
	return b
}
