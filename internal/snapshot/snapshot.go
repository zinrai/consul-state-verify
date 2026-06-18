// Package snapshot is the on-disk shape consul-state-dump writes and
// consul-state-diff reads. See README.md.
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
)

// Snapshot is one logical, per-node view of a Consul server cluster, keyed by
// the polled server's address.
type Snapshot struct {
	TakenAt string               `json:"taken_at"`
	Nodes   map[string]NodeState `json:"nodes"`
}

// NodeState is one polled server's local view: KV, service catalog, node
// catalog, and the Consul version live at dump time.
type NodeState struct {
	ConsulVersion string                  `json:"consul_version"`
	KV            map[string]KVEntry      `json:"kv"`
	Services      map[string]ServiceState `json:"services"`
	CatalogNodes  map[string]CatalogEntry `json:"catalog_nodes"`
}

// KVEntry is one key reduced to a hash, length, modify_index, and flags.
type KVEntry struct {
	SHA256      string `json:"sha256"`
	Len         int    `json:"len"`
	ModifyIndex int64  `json:"modify_index"`
	Flags       int    `json:"flags"`
}

// ServiceState is one service name's healthy (passing) instance count.
type ServiceState struct {
	Healthy int `json:"healthy"`
}

// CatalogEntry is one catalog node's address, id, and modify_index.
type CatalogEntry struct {
	Address     string `json:"address"`
	ID          string `json:"id"`
	ModifyIndex int64  `json:"modify_index"`
}

// Load reads a snapshot file and checks it is structurally a snapshot: valid JSON
// with at least one node, each carrying its three maps.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if len(s.Nodes) == 0 {
		return nil, fmt.Errorf("%s: not a consul-state snapshot (no nodes)", path)
	}
	for addr, n := range s.Nodes {
		if n.KV == nil || n.Services == nil || n.CatalogNodes == nil {
			return nil, fmt.Errorf("%s: node %q is missing kv/services/catalog_nodes", path, addr)
		}
	}
	return &s, nil
}
