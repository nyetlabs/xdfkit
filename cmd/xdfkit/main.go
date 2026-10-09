// Command xdfkit converts map definitions. So far it reads and writes WinOLS KP
// files, the canonical model JSON or YAML (docs/model.md) and TunerPro XDF
// with its metadata file, and writes CSV map lists.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"go.nyet.org/xdfkit/api"
	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/lint"
)

const usage = `usage:
  xdfkit [-f json|yaml|kp|csv|xdf] [--template orig.kp] [-i image] [-r ref]...
         [--meta file.meta.json] [--origin damos|a2l|hand|located]
         [--tuner categories.json] [--force] input [output]
  xdfkit verify file.json|file.yaml...
  xdfkit lint -i image [--family me7|m3] [--json findings.json] input
  xdfkit fix -i image (-o output | -n) [--rules R1,R2] [--only ID,...]
             [--min-confidence high|medium|low] [--findings findings.json] input
  xdfkit version
  xdfkit xdf2kp|kp2json FILE...
  xdfkit kp2xdf [-i image] [-m] FILE...

A single hyphen is for single-letter options, which also have long names:
-f --format, -i --image, -r --ref, -o --output, -n --dry-run, -m --write-meta.
Long options take two hyphens. Options may come before or after the files.

Converts between KP, XDF and the model JSON or YAML, and writes CSV; the
input format is detected from the contents, the output format comes from -f,
else the output extension (.yml is yaml), else json. YAML is the model JSON in
YAML syntax, with the same typed numbers and stamp. Input "-" reads stdin;
output other than KP without an output file goes to stdout. Existing output
files are not overwritten unless --force is given. KP output from JSON or YAML
fills the KP fields the model doesn't carry from --template (the KP file the
JSON came from reproduces it), else from defaults. CSV output is
mapdump's map list: -i adds the value ranges from the image, and each -r adds a
column with the names of the matching maps in that KP, JSON or YAML definition.
XDF output is TunerPro's format; -i adds the file region and the labels of
"subtract" axes, and the definition title is the output file's name. An XDF
output file comes with its metadata file, NAME.meta.json next to NAME.xdf,
holding what XDF can't express. XDF input reads the metadata file given by
--meta, else NAME.meta.json next to it if it exists; with it, an XDF unchanged
since it was written converts back to the definition exactly, and edits made
to it are reported. --origin records where a KP or XDF input's definitions
came from in the model's provenance: damos or a2l (exported from Bosch
data), hand (made by hand; the default, unless an XDF's metadata file has
another) or located (found in the image by a program such as me7info; never
a corpus source). --tuner keeps only the maps named in a category table (the
corpus categories.json) and the objects their axes point at, filed under the
table's categories.

xdf2kp, kp2xdf and kp2json convert each FILE to the same name with the .kp,
.xdf or .json extension, next to it, and reject input of another format. They
never replace an existing file. kp2xdf's -i is the flash image, as above; -m
also writes the metadata file, without which the XDF doesn't convert back
exactly. A link named kp2xdf (the release archives have them) is the same as
xdfkit kp2xdf.

verify checks each digest in a JSON or YAML file's stamp (RFC 8785 and jq -S .)
and prints clean, edited, mixed (the digests disagree), unknown or unstamped.

lint checks the KP file's image axes against the flash image and prints one
line per finding; --json also writes them as JSON ("-" for stdout). It exits 1
when there are findings. fix applies the findings that have a fix: by default
those of high confidence, or those chosen by --only, --rules and
--min-confidence. With --findings it applies the fixes in a (possibly edited)
lint --json file instead of linting. -o - writes to stdout; -n only reports.
`

func main() {
	log := func(err error) {
		if errors.Is(err, pflag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "xdfkit:", err)
		os.Exit(1)
	}
	if name, ok := alias(os.Args[0]); ok {
		os.Exit(runAlias(name, os.Args[1:]))
	}
	args := os.Args[1:]
	if len(args) > 0 {
		if _, ok := aliases[args[0]]; ok {
			os.Exit(runAlias(args[0], args[1:]))
		}
	}
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println(api.Version)
		return
	}
	cmds := map[string]func([]string) error{"verify": verify, "lint": lintCmd, "fix": fix}
	run := convert
	if len(args) > 0 && cmds[args[0]] != nil {
		run, args = cmds[args[0]], args[1:]
	}
	if err := run(args); err != nil {
		log(err)
	}
}

func newFlags(name string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	return fs
}

// parse parses options anywhere among the arguments and returns the others.
// One hyphen is for single-letter options only: a long option spelled with
// one (-force) is an error naming the right spelling, since pflag would
// read it as -f orce.
func parse(fs *pflag.FlagSet, args []string) ([]string, error) {
	for _, a := range args {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(a, "-"), "=")
		if len(a) > 2 && a[0] == '-' && a[1] != '-' && (fs.Lookup(name) != nil || name == "help" || name == "version") {
			return nil, fmt.Errorf("%s: long options take two hyphens: --%s", a, name)
		}
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return fs.Args(), nil
}

func list(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func lintCmd(args []string) error {
	fs := newFlags("lint")
	image := fs.StringP("image", "i", "", "flash image")
	family := fs.String("family", "", "ECU family: me7 or m3 (default: from the image)")
	jsonOut := fs.String("json", "", "also write the findings as JSON to this file")
	force := fs.Bool("force", false, "overwrite an existing JSON file")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *image == "" {
		fs.Usage()
		os.Exit(2)
	}
	data, err := readInput(pos[0])
	if err != nil {
		return err
	}
	img, err := os.ReadFile(*image)
	if err != nil {
		return err
	}
	resp, err := api.Lint(data, api.LintRequest{Image: img, Family: *family})
	warn(pos[0], resp.Warnings)
	if err != nil {
		return fmt.Errorf("%s: %w", pos[0], err)
	}
	if *jsonOut != "" {
		b, err := canon.Marshal(resp)
		if err != nil {
			return err
		}
		if err := writeOutput(*jsonOut, b, *force); err != nil {
			return err
		}
	}
	if *jsonOut != "-" {
		fmt.Printf("%s: family %s (%s), %d findings\n", pos[0], resp.Family, resp.FamilySource, len(resp.Findings))
		printFindings(os.Stdout, resp.Findings)
	}
	if len(resp.Findings) > 0 {
		os.Exit(1)
	}
	return nil
}

func fix(args []string) error {
	fs := newFlags("fix")
	image := fs.StringP("image", "i", "", "flash image")
	family := fs.String("family", "", "ECU family: me7 or m3 (default: from the image)")
	out := fs.StringP("output", "o", "", "output file")
	rules := fs.String("rules", "", "comma-separated rules to apply")
	only := fs.String("only", "", "comma-separated finding IDs to apply")
	minConf := fs.String("min-confidence", "", "lowest confidence to apply (default high)")
	findings := fs.String("findings", "", "apply the fixes in this lint --json file")
	dry := fs.BoolP("dry-run", "n", false, "report what would be fixed without writing")
	force := fs.Bool("force", false, "overwrite an existing output file")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || (*out == "") == !*dry || (*image == "" && *findings == "") {
		fs.Usage()
		os.Exit(2)
	}
	data, err := readInput(pos[0])
	if err != nil {
		return err
	}
	req := api.FixRequest{
		LintRequest: api.LintRequest{Family: *family},
		Rules:       list(*rules), Only: list(*only), MinConfidence: *minConf,
	}
	if *image != "" {
		if req.Image, err = os.ReadFile(*image); err != nil {
			return err
		}
	}
	if *findings != "" {
		b, err := readInput(*findings)
		if err != nil {
			return err
		}
		var lr api.LintResponse
		if err := canon.Unmarshal(b, &lr); err != nil {
			return fmt.Errorf("%s: %w", *findings, err)
		}
		min := *minConf
		if min == "" {
			min = lint.None
		}
		req.Findings = lint.Select(lr.Findings, req.Rules, req.Only, min)
		if req.Findings == nil {
			req.Findings = []lint.Finding{}
		}
	}
	b, resp, err := api.Fix(data, req)
	warn(pos[0], resp.Warnings)
	if err != nil {
		return fmt.Errorf("%s: %w", pos[0], err)
	}
	fmt.Fprintf(os.Stderr, "%s: %d fixes: %s\n", pos[0], len(resp.Applied), strings.Join(resp.Applied, " "))
	if len(resp.Remaining) > 0 {
		fmt.Fprintf(os.Stderr, "%s: %d findings remain:\n", pos[0], len(resp.Remaining))
		printFindings(os.Stderr, resp.Remaining)
	}
	if *dry {
		return nil
	}
	return writeOutput(*out, b, *force)
}

func printFindings(w io.Writer, fs []lint.Finding) {
	for _, f := range fs {
		maps := make([]string, len(f.Maps))
		for i, m := range f.Maps {
			n := m.ID
			if n == "" {
				n = strconv.Quote(m.Name)
			}
			if m.Axis != "" {
				n += "/" + m.Axis
			}
			maps[i] = n
		}
		fmt.Fprintf(w, "%s %s: %s; %s\n", f.ID, f.Confidence, f.Message, strings.Join(maps, " "))
	}
}

func warn(name string, ws []string) {
	for _, w := range ws {
		fmt.Fprintf(os.Stderr, "xdfkit: warning: %s: %s\n", name, w)
	}
}

func convert(args []string) error {
	fs := newFlags("xdfkit")
	format := fs.StringP("format", "f", "", "output format: json, yaml, kp, csv or xdf")
	force := fs.Bool("force", false, "overwrite an existing output file")
	template := fs.String("template", "", "KP file supplying the fields the model doesn't carry")
	image := fs.StringP("image", "i", "", "flash image, for CSV value ranges and XDF")
	meta := fs.String("meta", "", "metadata file of the XDF input (default: NAME.meta.json next to it)")
	origin := fs.String("origin", "", "where a KP or XDF input's definitions came from: damos, a2l, hand or located")
	tuner := fs.String("tuner", "", "category table: keep only its maps and their axes")
	refs := fs.StringArrayP("ref", "r", nil, "reference definition, for a CSV column of matching map names (repeatable)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 || len(pos) > 2 {
		fs.Usage()
		os.Exit(2)
	}
	in, out := pos[0], ""
	if len(pos) == 2 {
		out = pos[1]
	}
	if *format == "" {
		*format = strings.TrimPrefix(filepath.Ext(out), ".")
		switch *format {
		case "":
			*format = api.JSON
		case "yml":
			*format = api.YAML
		}
	}

	if *format == api.KP && out == "" {
		return errors.New("kp output needs an output file")
	}
	data, err := readInput(in)
	if err != nil {
		return err
	}
	name := filepath.Base(in)
	if in == "-" {
		name = ""
	}
	req := api.ConvertRequest{To: *format, Name: name, Origin: *origin, Title: xdfTitle(*format, out)}
	if *template != "" {
		if req.Template, err = os.ReadFile(*template); err != nil {
			return err
		}
	}
	if *image != "" {
		if req.Image, err = os.ReadFile(*image); err != nil {
			return err
		}
	}
	if *tuner != "" {
		if req.Tuner, err = os.ReadFile(*tuner); err != nil {
			return err
		}
	}
	for _, r := range *refs {
		b, err := os.ReadFile(r)
		if err != nil {
			return err
		}
		req.Refs = append(req.Refs, api.Ref{Name: r, Data: b})
	}
	if *meta != "" {
		req.Meta, err = os.ReadFile(*meta)
	} else if in != "-" && api.Detect(data) == api.XDF {
		req.Meta, err = readMeta(in)
	}
	if err != nil {
		return err
	}
	b, resp, err := api.Convert(data, req)
	warn(in, resp.Warnings)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	if out == "" {
		out = "-"
	}
	return writeConverted(out, b, resp.Meta, *force)
}

// metaName is the metadata file next to an XDF: NAME.meta.json for NAME.xdf.
func metaName(xdf string) string {
	return strings.TrimSuffix(xdf, filepath.Ext(xdf)) + ".meta.json"
}

// readMeta reads the metadata file next to an XDF, or nil if there is none.
func readMeta(xdf string) ([]byte, error) {
	b, err := os.ReadFile(metaName(xdf))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// xdfTitle is the definition title of XDF output to a file: the file's name.
func xdfTitle(format, out string) string {
	if format != api.XDF || out == "" || out == "-" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(out), filepath.Ext(out))
}

// writeConverted writes the output and, unless it goes to stdout, the
// metadata file next to it.
func writeConverted(out string, b, meta []byte, force bool) error {
	if meta == nil || out == "-" {
		return writeOutput(out, b, force)
	}
	if _, err := os.Stat(metaName(out)); err == nil && !force {
		return fmt.Errorf("%s exists (use --force)", metaName(out))
	}
	if err := writeOutput(out, b, force); err != nil {
		return err
	}
	return writeOutput(metaName(out), meta, force)
}

func verify(files []string) error {
	if len(files) == 0 {
		return errors.New("verify: no files")
	}
	bad := false
	for _, name := range files {
		data, err := readInput(name)
		if err != nil {
			return err
		}
		v, err := api.Verify(data)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		fmt.Printf("%s: %s\n", name, v)
		bad = bad || v.Status != "clean"
	}
	if bad {
		os.Exit(1)
	}
	return nil
}

func readInput(name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(name)
}

func writeOutput(name string, b []byte, force bool) error {
	if name == "-" {
		_, err := os.Stdout.Write(b)
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	w, err := os.OpenFile(name, flags, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s exists (use --force)", name)
	} else if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
