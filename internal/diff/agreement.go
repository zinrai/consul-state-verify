package diff

import (
	"fmt"
	"sort"
	"strings"
)

// checkAgreement is comparison A: inter-node agreement within one dump. See
// README.md.
func checkAgreement(nodes map[string]NodeState, scope string) []Finding {
	var findings []Finding
	names := nodeNames(nodes)
	have := func(holders int) bool { return holders < len(names) }

	for _, key := range unionKVKeys(nodes) {
		holders := kvHolders(nodes, names, key)
		if have(len(holders)) {
			findings = append(findings, partialFinding(scope, "kv", key, "present on", "nodes", len(holders), names, holderNames(holders)))
			continue
		}
		findings = append(findings, kvAgreementFindings(key, holders, scope)...)
	}

	for _, cnode := range unionCatalogNames(nodes) {
		holders := catalogHolders(nodes, names, cnode)
		if have(len(holders)) {
			findings = append(findings, partialFinding(scope, "node", cnode, "in catalog on", "servers", len(holders), names, catalogHolderNames(holders)))
			continue
		}
		findings = append(findings, nodeAgreementFindings(cnode, holders, scope)...)
	}

	for _, svc := range unionServiceNames(nodes) {
		holders := serviceHolders(nodes, names, svc)
		if have(len(holders)) {
			findings = append(findings, partialFinding(scope, "service", svc, "registered on", "nodes", len(holders), names, setOf(holders)))
		}
	}
	// healthy counts are deliberately not compared across nodes: health-check
	// transitions make momentary skew normal. A drop to zero healthy is judged
	// only by the outage rule in comparison B.
	return findings
}

// setOf turns a slice of names into a presence set.
func setOf(names []string) map[string]bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return set
}

func partialFinding(scope, kind, name, verb, unit string, held int, names []string, present map[string]bool) Finding {
	missing := missingFrom(names, present)
	return Finding{"INVESTIGATE", kind, name, fmt.Sprintf(
		"%s%s %d/%d %s (missing on %s); re-dump to see if it persists",
		scope, verb, held, len(names), unit, strings.Join(missing, ","))}
}

func holderNames(holders map[string]KVEntry) map[string]bool {
	present := map[string]bool{}
	for n := range holders {
		present[n] = true
	}
	return present
}

func catalogHolderNames(holders map[string]CatalogEntry) map[string]bool {
	present := map[string]bool{}
	for n := range holders {
		present[n] = true
	}
	return present
}

// kvAgreementFindings flags one key whose value splits between nodes at the same
// modify_index.
func kvAgreementFindings(key string, holders map[string]KVEntry, scope string) []Finding {
	var findings []Finding
	byIndex := map[int64]map[string][]string{} // modify_index -> sha256 -> nodes
	for n, e := range holders {
		if byIndex[e.ModifyIndex] == nil {
			byIndex[e.ModifyIndex] = map[string][]string{}
		}
		byIndex[e.ModifyIndex][e.SHA256] = append(byIndex[e.ModifyIndex][e.SHA256], n)
	}
	for _, mi := range sortedIndexes(byIndex) {
		shas := byIndex[mi]
		if len(shas) <= 1 {
			continue
		}
		findings = append(findings, Finding{"FAIL", "kv", key, fmt.Sprintf(
			"%svalue differs between nodes at the same modify_index=%d: %s",
			scope, mi, divergenceDetail(shas, 12))})
	}
	return findings
}

// nodeAgreementFindings flags one catalog node whose address/id splits between
// servers at the same modify_index.
func nodeAgreementFindings(cnode string, holders map[string]CatalogEntry, scope string) []Finding {
	var findings []Finding
	byIndex := map[int64]map[string][]string{} // modify_index -> "address id=..." -> servers
	for n, e := range holders {
		v := nodeValue(e)
		if byIndex[e.ModifyIndex] == nil {
			byIndex[e.ModifyIndex] = map[string][]string{}
		}
		byIndex[e.ModifyIndex][v] = append(byIndex[e.ModifyIndex][v], n)
	}
	for _, mi := range sortedIndexes(byIndex) {
		values := byIndex[mi]
		if len(values) <= 1 {
			continue
		}
		findings = append(findings, Finding{"FAIL", "node", cnode, fmt.Sprintf(
			"%saddress/id differs between servers at the same modify_index=%d: %s",
			scope, mi, divergenceDetail(values, 0))})
	}
	return findings
}

// divergenceDetail renders "which value on which nodes" for one modify_index.
// trunc>0 truncates each value to that many chars (used for sha256).
func divergenceDetail(byValue map[string][]string, trunc int) string {
	values := make([]string, 0, len(byValue))
	for v := range byValue {
		values = append(values, v)
	}
	sort.Strings(values)
	var parts []string
	for _, v := range values {
		label := v
		if trunc > 0 && len(v) > trunc {
			label = v[:trunc]
		}
		holders := append([]string{}, byValue[v]...)
		sort.Strings(holders)
		parts = append(parts, fmt.Sprintf("%s@%s", label, strings.Join(holders, "+")))
	}
	return strings.Join(parts, " vs ")
}

func sortedIndexes(m map[int64]map[string][]string) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
