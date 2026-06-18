// Package dump takes a logical, per-node snapshot of a Consul server cluster
// (KV, service catalog, node catalog). Read-only GETs, all with ?stale. See
// README.md.
package dump

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/zinrai/consul-state-verify/internal/snapshot"
)

// Cluster snapshots every address; if any fetch fails it returns an error and no
// snapshot (no partial dumps).
func Cluster(addrs []string, timeout time.Duration) (*snapshot.Snapshot, error) {
	snap := &snapshot.Snapshot{
		TakenAt: time.Now().Format(time.RFC3339),
		Nodes:   map[string]snapshot.NodeState{},
	}
	client := &http.Client{Timeout: timeout}
	for _, addr := range addrs {
		ns, err := (&fetcher{addr: addr, http: client}).node()
		if err != nil {
			return nil, fmt.Errorf("%s: %v", addr, err)
		}
		snap.Nodes[addr] = ns
	}
	return snap, nil
}

type httpStatus struct{ code int }

func (e *httpStatus) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

type fetcher struct {
	addr string
	http *http.Client
}

func (f *fetcher) getJSON(path string, out any) error {
	resp, err := f.http.Get("http://" + f.addr + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &httpStatus{resp.StatusCode}
	}
	return json.Unmarshal(body, out)
}

func statusOf(err error) int {
	if hs, ok := err.(*httpStatus); ok {
		return hs.code
	}
	return 0
}

func (f *fetcher) node() (snapshot.NodeState, error) {
	ver, err := f.version()
	if err != nil {
		return snapshot.NodeState{}, err
	}
	kv, err := f.kv()
	if err != nil {
		return snapshot.NodeState{}, err
	}
	svcs, err := f.services()
	if err != nil {
		return snapshot.NodeState{}, err
	}
	cnodes, err := f.catalogNodes()
	if err != nil {
		return snapshot.NodeState{}, err
	}
	return snapshot.NodeState{ConsulVersion: ver, KV: kv, Services: svcs, CatalogNodes: cnodes}, nil
}

func (f *fetcher) version() (string, error) {
	var self struct {
		Config struct {
			Version string `json:"Version"`
		} `json:"Config"`
	}
	if err := f.getJSON("/v1/agent/self", &self); err != nil {
		return "", err
	}
	return self.Config.Version, nil
}

// kv records each key as sha256 + length. A 404 means the store is empty.
func (f *fetcher) kv() (map[string]snapshot.KVEntry, error) {
	var entries []struct {
		Key         string `json:"Key"`
		Value       string `json:"Value"`
		ModifyIndex int64  `json:"ModifyIndex"`
		Flags       int    `json:"Flags"`
	}
	if err := f.getJSON("/v1/kv/?recurse&stale", &entries); err != nil {
		if statusOf(err) == http.StatusNotFound {
			return map[string]snapshot.KVEntry{}, nil
		}
		return nil, err
	}
	out := map[string]snapshot.KVEntry{}
	for _, e := range entries {
		raw, err := decodeValue(e.Value)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(raw)
		out[e.Key] = snapshot.KVEntry{
			SHA256: hex.EncodeToString(sum[:]), Len: len(raw), ModifyIndex: e.ModifyIndex, Flags: e.Flags,
		}
	}
	return out, nil
}

// decodeValue base64-decodes a KV value; an empty value is an empty payload.
func decodeValue(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// services records the healthy (passing) instance count per service name.
func (f *fetcher) services() (map[string]snapshot.ServiceState, error) {
	var catalog map[string][]string
	if err := f.getJSON("/v1/catalog/services?stale", &catalog); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(catalog))
	for n := range catalog {
		names = append(names, n)
	}
	sort.Strings(names)
	out := map[string]snapshot.ServiceState{}
	for _, name := range names {
		var instances []json.RawMessage
		if err := f.getJSON("/v1/health/service/"+url.PathEscape(name)+"?passing&stale", &instances); err != nil {
			return nil, err
		}
		out[name] = snapshot.ServiceState{Healthy: len(instances)}
	}
	return out, nil
}

// catalogNodes records every node in the catalog (client agents included).
func (f *fetcher) catalogNodes() (map[string]snapshot.CatalogEntry, error) {
	var nodes []struct {
		Node        string `json:"Node"`
		Address     string `json:"Address"`
		ID          string `json:"ID"`
		ModifyIndex int64  `json:"ModifyIndex"`
	}
	if err := f.getJSON("/v1/catalog/nodes?stale", &nodes); err != nil {
		return nil, err
	}
	out := map[string]snapshot.CatalogEntry{}
	for _, e := range nodes {
		out[e.Node] = snapshot.CatalogEntry{Address: e.Address, ID: e.ID, ModifyIndex: e.ModifyIndex}
	}
	return out, nil
}
