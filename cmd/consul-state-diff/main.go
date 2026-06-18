// Command consul-state-diff compares two consul-state-dump files. The comparison
// lives in internal/diff; this is the entry point.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zinrai/consul-state-verify/internal/diff"
)

// Build information, injected at release time by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: consul-state-diff <after.json>            (inter-node self-check)")
		fmt.Fprintln(os.Stderr, "       consul-state-diff <before.json> <after.json>")
	}
	flag.Parse()
	if *showVersion {
		fmt.Printf("consul-state-diff %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	var code int
	var err error
	switch flag.NArg() {
	case 1:
		code, err = diff.SelfCheckFile(flag.Arg(0))
	case 2:
		code, err = diff.CompareFiles(flag.Arg(0), flag.Arg(1))
	default:
		flag.Usage()
		os.Exit(64)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "consul-state-diff: %v\n", err)
		os.Exit(64)
	}
	os.Exit(code)
}
