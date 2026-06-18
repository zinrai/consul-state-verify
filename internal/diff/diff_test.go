package diff

import (
	"strings"
	"testing"
)

func kv(mi int64, sha string) KVEntry { return KVEntry{SHA256: sha, Len: 5, ModifyIndex: mi} }
func cn(addr, id string, mi int64) CatalogEntry {
	return CatalogEntry{Address: addr, ID: id, ModifyIndex: mi}
}
func svc(h int) ServiceState { return ServiceState{Healthy: h} }

func find(t *testing.T, fs []Finding, level, kind, name string) Finding {
	t.Helper()
	for _, f := range fs {
		if f.Level == level && f.Kind == kind && f.Name == name {
			return f
		}
	}
	t.Fatalf("expected %s %s %q; got %+v", level, kind, name, fs)
	return Finding{}
}

func absent(t *testing.T, fs []Finding, kind, name string) {
	t.Helper()
	for _, f := range fs {
		if f.Kind == kind && f.Name == name && f.Level != "INFO" {
			t.Fatalf("did not expect a finding for %s %q; got %+v", kind, name, f)
		}
	}
}

// Comparison A: a value split at the same modify_index is FSM divergence (FAIL);
// partial presence is INVESTIGATE; full agreement produces no finding.
func TestAgreement(t *testing.T) {
	nodes := map[string]NodeState{
		"n1": {KV: map[string]KVEntry{"agree": kv(10, "a"), "split": kv(20, "x"), "part": kv(30, "p")}},
		"n2": {KV: map[string]KVEntry{"agree": kv(10, "a"), "split": kv(20, "y")}},
	}
	fs := checkAgreement(nodes, "")
	if f := find(t, fs, "FAIL", "kv", "split"); !strings.Contains(f.Detail, "modify_index=20") {
		t.Errorf("split detail: %s", f.Detail)
	}
	find(t, fs, "INVESTIGATE", "kv", "part")
	absent(t, fs, "kv", "agree")
}

// Comparison B for KV: same index different value is corruption (FAIL); a lower
// index is a regression (FAIL); a higher index is a legitimate rewrite (counted,
// no finding); a full loss is reported to investigate.
func TestDiffKV(t *testing.T) {
	before := map[string]NodeState{"n1": {KV: map[string]KVEntry{
		"corrupt": kv(10, "a"), "regress": kv(50, "b"), "rewrite": kv(10, "c"), "lost": kv(11, "d"),
	}}}
	after := map[string]NodeState{"n1": {KV: map[string]KVEntry{
		"corrupt": kv(10, "aX"), "regress": kv(20, "b"), "rewrite": kv(99, "cX"),
	}}}
	fs := diffKV(buildBaseline(before), after)

	find(t, fs, "FAIL", "kv", "corrupt")
	find(t, fs, "FAIL", "kv", "regress")
	absent(t, fs, "kv", "rewrite") // advanced index: counted, no finding
	find(t, fs, "INVESTIGATE", "kv", "lost")
	if s := find(t, fs, "INFO", "kv", "(summary)").Detail; !strings.Contains(s, "1 keys legitimately rewritten") {
		t.Errorf("summary: %s", s)
	}
}

// Comparison B for services: a full loss is reported to investigate; a service
// still registered everywhere but dropped to zero healthy is an outage FAIL; a
// service that kept its instances produces no finding.
func TestDiffServices(t *testing.T) {
	before := map[string]NodeState{"n1": {Services: map[string]ServiceState{
		"ok": svc(2), "zero": svc(2), "gone": svc(2),
	}}}
	after := map[string]NodeState{"n1": {Services: map[string]ServiceState{
		"ok": svc(2), "zero": svc(0),
	}}}
	fs := diffServices(buildBaseline(before), after)

	absent(t, fs, "service", "ok")
	find(t, fs, "FAIL", "service", "zero")        // healthy dropped to 0
	find(t, fs, "INVESTIGATE", "service", "gone") // missing entirely
}

// Comparison B for catalog nodes: a same-named node whose id changed with an
// advanced index is a legitimate re-registration (counted); the same index with
// a different id is corruption (FAIL); a full loss is reported to investigate.
func TestDiffCatalogNodes(t *testing.T) {
	before := map[string]NodeState{"n1": {CatalogNodes: map[string]CatalogEntry{
		"rereg": cn("1.1.1.1", "old", 10), "corrupt": cn("1.1.1.2", "x", 20), "lost": cn("1.1.1.3", "y", 30),
	}}}
	after := map[string]NodeState{"n1": {CatalogNodes: map[string]CatalogEntry{
		"rereg": cn("1.1.1.1", "new", 200), "corrupt": cn("1.1.1.2", "xX", 20),
	}}}
	fs := diffCatalogNodes(buildBaseline(before), after)

	absent(t, fs, "node", "rereg") // re-registration: counted, no finding
	find(t, fs, "FAIL", "node", "corrupt")
	find(t, fs, "INVESTIGATE", "node", "lost")
	if s := find(t, fs, "INFO", "node", "(summary)").Detail; !strings.Contains(s, "1 nodes re-registered") {
		t.Errorf("summary: %s", s)
	}
}
