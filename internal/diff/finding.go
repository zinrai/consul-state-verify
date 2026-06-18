package diff

import (
	"fmt"
	"sort"
	"strings"
)

// Finding is one line of the verdict: a level, the kind/name it concerns, and
// the human-readable grounds.
type Finding struct {
	Level  string // FAIL, INVESTIGATE, INFO
	Kind   string // kv, service, node, nodes
	Name   string
	Detail string
}

var levelOrder = map[string]int{"FAIL": 0, "INVESTIGATE": 1, "INFO": 2}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if levelOrder[a.Level] != levelOrder[b.Level] {
			return levelOrder[a.Level] < levelOrder[b.Level]
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
}

func countLevels(findings []Finding) (fails, invs int) {
	for _, f := range findings {
		switch f.Level {
		case "FAIL":
			fails++
		case "INVESTIGATE":
			invs++
		}
	}
	return
}

// render writes the report to stdout and returns the exit code.
func render(findings []Finding, header []string) int {
	const rule = "--------------------------------------------------------------------------------------"
	out := append([]string{}, header...)
	out = append(out, rule)
	sortFindings(findings)
	for _, f := range findings {
		out = append(out, fmt.Sprintf("%-11s %-8s %s  %s", f.Level, f.Kind, f.Name, f.Detail))
	}
	out = append(out, rule)

	fails, invs := countLevels(findings)
	code := 0
	switch {
	case fails > 0:
		out = append(out, fmt.Sprintf("FAIL: %d violation(s), %d to review", fails, invs))
		code = 2
	case invs > 0:
		out = append(out, fmt.Sprintf("INVESTIGATE: %d loss(es) to review; read the report and record the judgment", invs))
		code = 1
	default:
		out = append(out, "PASS: no loss, no inter-node mismatch")
	}
	fmt.Println(strings.Join(out, "\n"))
	return code
}
