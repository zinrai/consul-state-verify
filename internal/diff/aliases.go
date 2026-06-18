package diff

import "github.com/zinrai/consul-state-verify/internal/snapshot"

// The format lives in the snapshot package. These aliases let the comparison code
// in this package name the types without qualification.
type (
	NodeState    = snapshot.NodeState
	KVEntry      = snapshot.KVEntry
	ServiceState = snapshot.ServiceState
	CatalogEntry = snapshot.CatalogEntry
)
