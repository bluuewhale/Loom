package graph

// NodeRegistry maps human-readable string node names to internal NodeID integers
// and back, enabling callers to build graphs using string identifiers while core
// algorithms continue to use fast integer NodeIDs.
//
// Forward lookup (name -> NodeID) is O(1) via a map.
// Reverse lookup (NodeID -> name) is O(1) via a slice (NodeID is the index).
//
// NodeRegistry is not safe for concurrent use; callers that require concurrency
// must provide external synchronization.
type NodeRegistry struct {
	names map[string]NodeID
	ids   []string
}

// NewRegistry returns a new, empty NodeRegistry ready for use.
func NewRegistry() *NodeRegistry {
	return &NodeRegistry{
		names: make(map[string]NodeID),
	}
}

// Register assigns a NodeID to name and returns it. If name was already
// registered, the existing NodeID is returned unchanged (idempotent).
// IDs are assigned sequentially starting from 0.
func (r *NodeRegistry) Register(name string) NodeID {
	if id, ok := r.names[name]; ok {
		return id
	}
	id := NodeID(len(r.ids))
	r.ids = append(r.ids, name)
	r.names[name] = id
	return id
}

// ID returns the NodeID for name and true if name is registered,
// or (0, false) if name has not been registered.
func (r *NodeRegistry) ID(name string) (NodeID, bool) {
	id, ok := r.names[name]
	return id, ok
}

// Name returns the string name for id and true if id is a valid NodeID,
// or ("", false) if id is out of range (including negative values).
func (r *NodeRegistry) Name(id NodeID) (string, bool) {
	if id < 0 || int(id) >= len(r.ids) {
		return "", false
	}
	return r.ids[id], true
}

// Len returns the number of distinct names registered.
func (r *NodeRegistry) Len() int {
	return len(r.ids)
}
