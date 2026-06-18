package diff

import "sort"

// --- shared lookups over the per-node view ---

func nodeNames(nodes map[string]NodeState) []string {
	names := make([]string, 0, len(nodes))
	for n := range nodes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// unionKVKeys / unionServiceNames / unionCatalogNames return every name seen on
// any node, sorted.
func unionKVKeys(nodes map[string]NodeState) []string {
	set := map[string]bool{}
	for _, ns := range nodes {
		for k := range ns.KV {
			set[k] = true
		}
	}
	return sortedSet(set)
}

func unionServiceNames(nodes map[string]NodeState) []string {
	set := map[string]bool{}
	for _, ns := range nodes {
		for s := range ns.Services {
			set[s] = true
		}
	}
	return sortedSet(set)
}

func unionCatalogNames(nodes map[string]NodeState) []string {
	set := map[string]bool{}
	for _, ns := range nodes {
		for c := range ns.CatalogNodes {
			set[c] = true
		}
	}
	return sortedSet(set)
}

// kvHolders returns node -> entry for the nodes that hold this key.
func kvHolders(nodes map[string]NodeState, names []string, key string) map[string]KVEntry {
	holders := map[string]KVEntry{}
	for _, n := range names {
		if e, ok := nodes[n].KV[key]; ok {
			holders[n] = e
		}
	}
	return holders
}

// serviceHolders returns the nodes on which this service name is registered.
func serviceHolders(nodes map[string]NodeState, names []string, svc string) []string {
	var holders []string
	for _, n := range names {
		if _, ok := nodes[n].Services[svc]; ok {
			holders = append(holders, n)
		}
	}
	return holders
}

// catalogHolders returns server -> entry for the servers whose catalog holds this
// node name.
func catalogHolders(nodes map[string]NodeState, names []string, cnode string) map[string]CatalogEntry {
	holders := map[string]CatalogEntry{}
	for _, n := range names {
		if e, ok := nodes[n].CatalogNodes[cnode]; ok {
			holders[n] = e
		}
	}
	return holders
}

// nodeValue is the part of a catalog entry that must agree: where the node is and
// which identity it carries.
func nodeValue(e CatalogEntry) string {
	return e.Address + " id=" + e.ID
}

// minHealthy is the lowest healthy instance count any node reports for a service.
func minHealthy(nodes map[string]NodeState, names []string, svc string) int {
	lowest := -1
	for _, n := range names {
		count := nodes[n].Services[svc].Healthy
		if lowest < 0 || count < lowest {
			lowest = count
		}
	}
	return lowest
}

func newestKV(nodes map[string]NodeState, names []string, key string) KVEntry {
	var newest KVEntry
	first := true
	for _, n := range names {
		e, ok := nodes[n].KV[key]
		if !ok {
			continue
		}
		if first || e.ModifyIndex > newest.ModifyIndex {
			newest = e
			first = false
		}
	}
	return newest
}

func newestCatalog(nodes map[string]NodeState, names []string, cnode string) CatalogEntry {
	var newest CatalogEntry
	first := true
	for _, n := range names {
		e, ok := nodes[n].CatalogNodes[cnode]
		if !ok {
			continue
		}
		if first || e.ModifyIndex > newest.ModifyIndex {
			newest = e
			first = false
		}
	}
	return newest
}

// missingFrom returns the sorted names present in all but absent from holders.
func missingFrom(all []string, holders map[string]bool) []string {
	var missing []string
	for _, n := range all {
		if !holders[n] {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	return missing
}
