package diff

import (
	"fmt"
	"sort"
	"strings"
)

// baseline is what was present on every before node. Entries on only some before
// nodes are already flagged by the agreement check on the before dump and are
// excluded here.
type baseline struct {
	kv     map[string]KVEntry
	svcs   map[string]bool
	counts map[string]int
	cnodes map[string]CatalogEntry
}

func buildBaseline(nodes map[string]NodeState) baseline {
	names := nodeNames(nodes)
	b := baseline{
		kv:     map[string]KVEntry{},
		svcs:   map[string]bool{},
		counts: map[string]int{},
		cnodes: map[string]CatalogEntry{},
	}

	for key := range commonKV(nodes, names) {
		b.kv[key] = newestKV(nodes, names, key)
	}
	for svc := range commonServices(nodes, names) {
		b.svcs[svc] = true
		b.counts[svc] = minHealthy(nodes, names, svc)
	}
	for cnode := range commonCatalog(nodes, names) {
		b.cnodes[cnode] = newestCatalog(nodes, names, cnode)
	}
	return b
}

func commonKV(nodes map[string]NodeState, names []string) map[string]bool {
	count := map[string]int{}
	for _, n := range names {
		for k := range nodes[n].KV {
			count[k]++
		}
	}
	return seenOnAll(count, len(names))
}

func commonServices(nodes map[string]NodeState, names []string) map[string]bool {
	count := map[string]int{}
	for _, n := range names {
		for s := range nodes[n].Services {
			count[s]++
		}
	}
	return seenOnAll(count, len(names))
}

func commonCatalog(nodes map[string]NodeState, names []string) map[string]bool {
	count := map[string]int{}
	for _, n := range names {
		for c := range nodes[n].CatalogNodes {
			count[c]++
		}
	}
	return seenOnAll(count, len(names))
}

// seenOnAll returns the names counted on every node (count == total).
func seenOnAll(count map[string]int, total int) map[string]bool {
	common := map[string]bool{}
	for name, c := range count {
		if c == total {
			common[name] = true
		}
	}
	return common
}

// classify compares each node's copy of one entry against the before value, by
// modify_index.
func classify(names []string, baseMI int64, baseVal string, present map[string]string, presentMI map[string]int64) (corrupt, regressed []string, advanced bool) {
	for _, n := range names {
		mi := presentMI[n]
		if mi == baseMI && present[n] != baseVal {
			corrupt = append(corrupt, n)
		}
		if mi < baseMI {
			regressed = append(regressed, n)
		}
		if mi > baseMI {
			advanced = true
		}
	}
	return
}

// diffKV is comparison B for KV: full loss, corruption, and regression against
// the baseline.
func diffKV(base baseline, after map[string]NodeState) []Finding {
	var findings []Finding
	names := nodeNames(after)
	rewritten := 0
	for _, key := range sortedKVKeys(base.kv) {
		b := base.kv[key]
		present := kvHolders(after, names, key)
		if len(present) == 0 {
			findings = append(findings, Finding{"INVESTIGATE", "kv", key, fmt.Sprintf(
				"missing on all nodes (before: modify_index=%d len=%d)", b.ModifyIndex, b.Len)})
			continue
		}
		if len(present) < len(names) {
			continue // already flagged by the agreement check on the after dump
		}
		val := map[string]string{}
		mi := map[string]int64{}
		for n, e := range present {
			val[n], mi[n] = e.SHA256, e.ModifyIndex
		}
		corrupt, regressed, advanced := classify(names, b.ModifyIndex, b.SHA256, val, mi)
		if len(corrupt) > 0 {
			findings = append(findings, Finding{"FAIL", "kv", key, fmt.Sprintf(
				"value differs from before at unchanged modify_index=%d on %s", b.ModifyIndex, strings.Join(corrupt, ","))})
		}
		if len(regressed) > 0 {
			findings = append(findings, Finding{"FAIL", "kv", key, fmt.Sprintf(
				"modify_index regressed below before=%d on %s", b.ModifyIndex, strings.Join(regressed, ","))})
		}
		if len(corrupt) == 0 && len(regressed) == 0 && advanced {
			rewritten++
		}
	}
	added := addedCount(unionKVKeys(after), func(k string) bool { _, ok := base.kv[k]; return ok })
	findings = append(findings, Finding{"INFO", "kv", "(summary)", fmt.Sprintf(
		"%d keys added, %d keys legitimately rewritten during the window", added, rewritten)})
	return findings
}

// diffCatalogNodes is comparison B for catalog nodes: full loss, corruption, and
// re-registration against the baseline.
func diffCatalogNodes(base baseline, after map[string]NodeState) []Finding {
	var findings []Finding
	names := nodeNames(after)
	reregistered := 0
	for _, cnode := range sortedCatalogKeys(base.cnodes) {
		b := base.cnodes[cnode]
		present := catalogHolders(after, names, cnode)
		if len(present) == 0 {
			findings = append(findings, Finding{"INVESTIGATE", "node", cnode, fmt.Sprintf(
				"missing from the catalog on all servers (before: %s)", nodeValue(b))})
			continue
		}
		if len(present) < len(names) {
			continue
		}
		val := map[string]string{}
		mi := map[string]int64{}
		for n, e := range present {
			val[n], mi[n] = nodeValue(e), e.ModifyIndex
		}
		corrupt, regressed, advanced := classify(names, b.ModifyIndex, nodeValue(b), val, mi)
		if len(corrupt) > 0 {
			findings = append(findings, Finding{"FAIL", "node", cnode, fmt.Sprintf(
				"address/id differs from before at unchanged modify_index=%d on %s", b.ModifyIndex, strings.Join(corrupt, ","))})
		}
		if len(regressed) > 0 {
			findings = append(findings, Finding{"FAIL", "node", cnode, fmt.Sprintf(
				"modify_index regressed below before=%d on %s", b.ModifyIndex, strings.Join(regressed, ","))})
		}
		if len(corrupt) == 0 && len(regressed) == 0 && advanced {
			reregistered++
		}
	}
	added := addedCount(unionCatalogNames(after), func(c string) bool { _, ok := base.cnodes[c]; return ok })
	findings = append(findings, Finding{"INFO", "node", "(summary)", fmt.Sprintf(
		"%d nodes added, %d nodes re-registered during the window", added, reregistered)})
	return findings
}

// diffServices is comparison B for services: full loss, and a drop to zero
// healthy instances (outage), against the baseline.
func diffServices(base baseline, after map[string]NodeState) []Finding {
	var findings []Finding
	names := nodeNames(after)

	for _, svc := range sortedSet(base.svcs) {
		holders := serviceHolders(after, names, svc)
		if len(holders) == 0 {
			findings = append(findings, Finding{"INVESTIGATE", "service", svc, fmt.Sprintf(
				"missing on all nodes (before healthy=%d)", base.counts[svc])})
			continue
		}
		if len(holders) < len(names) {
			continue // partial presence is the agreement check's finding
		}
		if base.counts[svc] >= 1 && minHealthy(after, names, svc) < 1 {
			findings = append(findings, Finding{"FAIL", "service", svc, fmt.Sprintf(
				"0 healthy instances after the window (before: %d)", base.counts[svc])})
		}
	}

	added := addedCount(unionServiceNames(after), func(s string) bool { return base.svcs[s] })
	findings = append(findings, Finding{"INFO", "service", "(summary)", fmt.Sprintf(
		"%d services added during the window", added)})
	return findings
}

func addedCount(union []string, inBaseline func(string) bool) int {
	n := 0
	for _, name := range union {
		if !inBaseline(name) {
			n++
		}
	}
	return n
}

func sortedKVKeys(m map[string]KVEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCatalogKeys(m map[string]CatalogEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
