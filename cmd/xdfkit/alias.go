package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
// Options are single letters after one dash; long options take two.
func runAlias(name string, args []string) int {
	from, to := aliases[name][0], aliases[name][1]
	usage := func(rc int) int {
		opts, help := "", ""
		if to == api.XDF {
			opts = "[-i image] [-m] "
			help = "-i adds the file region and the labels of \"subtract\" axes from the flash\nimage. -m also writes FILE.meta.json, without which the XDF doesn't convert\nback exactly.\n"
		}
		fmt.Fprintf(os.Stderr, "usage: %s %sFILE.%s...\n\nWrites FILE.%s next to each input. An existing file is not replaced.\n%s", name, opts, from, to, help)
		return rc
	}
	var opt aliasOpts
	var files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			files = append(files, args[i+1:]...)
			i = len(args)
		case a == "-h" || a == "--help":
			return usage(0)
		case a == "--version":
			fmt.Println(api.Version)
			return 0
		case a == "-i" && to == api.XDF:
			if i++; i == len(args) {
				fmt.Fprintf(os.Stderr, "%s: -i needs a file\n", name)
				return usage(2)
			}
			var err error
			if opt.image, err = os.ReadFile(args[i]); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
				return 1
			}
		case a == "-m" && to == api.XDF:
			opt.meta = true
		case len(a) > 1 && a[0] == '-':
			fmt.Fprintf(os.Stderr, "%s: unknown option %s\n", name, a)
			return usage(2)
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		return usage(2)
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
