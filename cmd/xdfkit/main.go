// Command xdfkit converts map definitions. So far it reads and writes WinOLS KP
// files and their JSON dump (kp.File's encoding, not yet the canonical model).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/kp"
)

// version is set from git describe by the Makefile. Do not edit it here.
var version = "dev"

const usage = `usage:
  xdfkit [-f json|kp] [-force] input [output]
  xdfkit verify file.json...
  xdfkit version

Converts between KP and its JSON dump; the input format is detected from the
contents, the output format comes from -f, else the output extension, else json.
Input "-" reads stdin; JSON output without an output file goes to stdout.
Existing output files are not overwritten unless -force is given.

verify checks each digest in a JSON file's stamp (RFC 8785 and jq -S .) and
prints clean, edited, mixed (the digests disagree), unknown or unstamped.
`

func main() {
	log := func(err error) {
		fmt.Fprintln(os.Stderr, "xdfkit:", err)
		os.Exit(1)
	}
	args := os.Args[1:]
	if len(args) == 1 && (args[0] == "version" || args[0] == "-version" || args[0] == "--version") {
		fmt.Println(version)
		return
	}
	if len(args) > 0 && args[0] == "verify" {
		if err := verify(args[1:]); err != nil {
			log(err)
		}
		return
	}
	if err := convert(args); err != nil {
		log(err)
	}
}

func convert(args []string) error {
	fs := flag.NewFlagSet("xdfkit", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	format := fs.String("f", "", "output format: json or kp")
	force := fs.Bool("force", false, "overwrite an existing output file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		fs.Usage()
		os.Exit(2)
	}
	in, out := fs.Arg(0), fs.Arg(1)
	if *format == "" {
		*format = strings.TrimPrefix(filepath.Ext(out), ".")
		if *format == "" {
			*format = "json"
		}
	}

	data, err := readInput(in)
	if err != nil {
		return err
	}
	f, err := load(in, data)
	if err != nil {
		return err
	}

	var b []byte
	switch *format {
	case "json":
		b, err = canon.MarshalStamped(f, "xdfkit "+version)
	case "kp":
		if out == "" {
			return errors.New("kp output needs an output file")
		}
		b, err = f.Encode()
	default:
		return fmt.Errorf("unknown output format %q", *format)
	}
	if err != nil {
		return err
	}
	if out == "" {
		_, err = os.Stdout.Write(b)
		return err
	}
	return writeOutput(out, b, *force)
}

// load parses a KP file or a JSON dump of one, warning about hand-edited JSON.
func load(name string, data []byte) (*kp.File, error) {
	if !bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n"), []byte("{")) {
		f, err := kp.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return f, nil
	}
	if s, checks, err := canon.Verify(data); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	} else if s != canon.Clean && s != canon.Unstamped {
		fmt.Fprintf(os.Stderr, "xdfkit: warning: %s: stamp %s\n", name, describe(s, checks))
	}
	var f kp.File
	if err := canon.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &f, nil
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
		s, checks, err := canon.Verify(data)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		fmt.Printf("%s: %s\n", name, describe(s, checks))
		bad = bad || s != canon.Clean
	}
	if bad {
		os.Exit(1)
	}
	return nil
}

// describe renders an overall status with the per-digest results.
func describe(s canon.Status, checks []canon.Check) string {
	var parts []string
	for _, c := range checks {
		parts = append(parts, fmt.Sprintf("%s %s", c.Canon, c.Status))
	}
	if len(parts) == 0 {
		return s.String()
	}
	return fmt.Sprintf("%s (%s)", s, strings.Join(parts, ", "))
}

func readInput(name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(name)
}

func writeOutput(name string, b []byte, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	w, err := os.OpenFile(name, flags, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s exists (use -force)", name)
	} else if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
