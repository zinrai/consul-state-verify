// Package diff compares two consul-state-dump files and writes the verdict. Pure
// computation over local files; it never talks to Consul. See README.md.
package diff

import (
	"fmt"

	"github.com/zinrai/consul-state-verify/internal/snapshot"
)

// SelfCheckFile runs the inter-node agreement check on one dump.
func SelfCheckFile(path string) (int, error) {
	s, err := snapshot.Load(path)
	if err != nil {
		return 0, err
	}
	findings := checkAgreement(s.Nodes, "")
	header := []string{fmt.Sprintf("consul-state-diff  self-check  %s (taken %s)", path, takenAt(s))}
	return render(findings, header), nil
}

// CompareFiles judges an after dump against a before dump.
func CompareFiles(beforePath, afterPath string) (int, error) {
	before, err := snapshot.Load(beforePath)
	if err != nil {
		return 0, err
	}
	after, err := snapshot.Load(afterPath)
	if err != nil {
		return 0, err
	}

	findings := checkAgreement(after.Nodes, "")
	findings = append(checkAgreement(before.Nodes, "(before dump) "), findings...)
	if !sameNodeSet(before.Nodes, after.Nodes) {
		findings = append(findings, Finding{"INFO", "nodes", "(set)", "before and after dumps cover different node sets"})
	}

	base := buildBaseline(before.Nodes)
	findings = append(findings, diffKV(base, after.Nodes)...)
	findings = append(findings, diffCatalogNodes(base, after.Nodes)...)
	findings = append(findings, diffServices(base, after.Nodes)...)

	header := []string{
		fmt.Sprintf("consul-state-diff  before=%s (taken %s)  after=%s (taken %s)",
			beforePath, takenAt(before), afterPath, takenAt(after)),
		fmt.Sprintf("nodes: %d (before) / %d (after)", len(before.Nodes), len(after.Nodes)),
	}
	return render(findings, header), nil
}

func takenAt(s *snapshot.Snapshot) string {
	if s.TakenAt == "" {
		return "?"
	}
	return s.TakenAt
}

func sameNodeSet(a, b map[string]NodeState) bool {
	if len(a) != len(b) {
		return false
	}
	for n := range a {
		if _, ok := b[n]; !ok {
			return false
		}
	}
	return true
}
