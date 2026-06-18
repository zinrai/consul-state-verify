// Command consul-state-dump writes one per-node snapshot of a Consul server
// cluster to stdout. The fetching lives in internal/dump; this is the entry point.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zinrai/consul-state-verify/internal/dump"
)

// Build information, injected at release time by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	timeout := flag.Duration("timeout", 5*time.Second, "per-request HTTP timeout")
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: consul-state-dump [--timeout d] <host:port> [<host:port> ...] > out.json")
	}
	flag.Parse()
	if *showVersion {
		fmt.Printf("consul-state-dump %s (commit %s, built %s)\n", version, commit, date)
		return
	}
	addrs := flag.Args()
	if len(addrs) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	for _, a := range addrs {
		if strings.Contains(a, ",") {
			fmt.Fprintf(os.Stderr, "consul-state-dump: addresses are space-separated, not comma-separated: %q\n", a)
			os.Exit(2)
		}
	}

	snap, err := dump.Cluster(addrs, *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "consul-state-dump: %v\n", err)
		fmt.Fprintln(os.Stderr, "no dump written: partial dumps are not emitted")
		os.Exit(1)
	}
	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "consul-state-dump: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
