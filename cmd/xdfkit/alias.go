package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"go.nyet.org/xdfkit/api"
)

// aliases are the names xdfkit answers to through a symlink: the input
// format each accepts and the format it writes. make build links them.
var aliases = map[string][2]string{
	"xdf2kp":  {api.XDF, api.KP},
	"kp2xdf":  {api.KP, api.XDF},
	"kp2json": {api.KP, api.JSON},
}

// alias is the alias argv0 names, if any.
func alias(argv0 string) (string, bool) {
	name := strings.TrimSuffix(argv0[strings.LastIndexAny(argv0, `/\`)+1:], ".exe")
	_, ok := aliases[name]
	return name, ok
}

// aliasOpts are the options of an alias writing XDF: the flash image, and
// whether to write the metadata file.
type aliasOpts struct {
	image []byte
	meta  bool
}

// runAlias converts each file to the same name with the new extension, next
// to it. An existing file is never replaced. An XDF input reads
// NAME.meta.json beside it when present; XDF output writes one only with -m.
func runAlias(name string, args []string) int {
	from, to := aliases[name][0], aliases[name][1]
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	version := fs.Bool("version", false, "print the version")
	var opt aliasOpts
	var image, opts, help string
	if to == api.XDF {
		fs.StringVarP(&image, "image", "i", "", "flash image")
		fs.BoolVarP(&opt.meta, "write-meta", "m", false, "also write the metadata file")
		opts = "[-i image] [-m] "
		help = "-i (--image) adds the file region and the labels of \"subtract\" axes from\nthe flash image. -m (--write-meta) also writes FILE.meta.json, without which\nthe XDF doesn't convert back exactly.\n"
	}
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s %sFILE.%s...\n\nWrites FILE.%s next to each input. An existing file is not replaced.\n%s", name, opts, from, to, help)
	}
	files, err := parse(fs, args)
	switch {
	case errors.Is(err, pflag.ErrHelp):
		return 0
	case err != nil:
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		fs.Usage()
		return 2
	case *version:
		fmt.Println(api.Version)
		return 0
	case len(files) == 0:
		fs.Usage()
		return 2
	}
	if image != "" {
		if opt.image, err = os.ReadFile(image); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			return 1
		}
	}
	rc := 0
	for _, in := range files {
		outs, err := aliasFile(name, in, from, to, opt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", name, in, err)
			rc = 1
			continue
		}
		fmt.Printf("%s -> %s\n", in, strings.Join(outs, ", "))
	}
	return rc
}

func aliasFile(name, in, from, to string, opt aliasOpts) ([]string, error) {
	data, err := os.ReadFile(in)
	if err != nil {
		return nil, err
	}
	if got := api.Detect(data); got == api.YAML {
		return nil, fmt.Errorf("is not %s", from)
	} else if got != from {
		return nil, fmt.Errorf("is %s, not %s", got, from)
	}
	out := strings.TrimSuffix(in, filepath.Ext(in)) + "." + to
	outs := []string{out}
	if opt.meta {
		outs = append(outs, metaName(out))
	}
	for _, o := range outs {
		if _, err := os.Stat(o); err == nil {
			return nil, fmt.Errorf("%s exists; not replacing it", o)
		}
	}
	req := api.ConvertRequest{To: to, Name: filepath.Base(in), Title: xdfTitle(to, out), Image: opt.image}
	if from == api.XDF {
		if req.Meta, err = readMeta(in); err != nil {
			return nil, err
		}
	}
	b, resp, err := api.Convert(data, req)
	for _, w := range resp.Warnings {
		fmt.Fprintf(os.Stderr, "%s: warning: %s: %s\n", name, in, w)
	}
	if err != nil {
		return nil, err
	}
	if !opt.meta {
		resp.Meta = nil
	}
	return outs, writeConverted(out, b, resp.Meta, false)
}
