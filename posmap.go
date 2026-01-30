package pathoram

// PositionMap tracks block-to-leaf assignments.
// For recursive ORAM, this can be implemented as another ORAM instance.
type PositionMap interface {
	// Get returns the leaf position for blockID.
	// Returns (leaf, true) if found, (0, false) if not.
	Get(blockID int) (leaf int, exists bool)

	// Set assigns blockID to leaf.
	Set(blockID int, leaf int)

	// Size returns the number of blocks with assigned positions.
	Size() int
}

// InMemoryPositionMap implements PositionMap using a Go map.
type InMemoryPositionMap struct {
	m map[int]int
}

// NewInMemoryPositionMap creates a new empty position map.
func NewInMemoryPositionMap() *InMemoryPositionMap {
	return &InMemoryPositionMap{
		m: make(map[int]int),
	}
}

// Get returns the leaf position for blockID.
func (p *InMemoryPositionMap) Get(blockID int) (int, bool) {
	leaf, ok := p.m[blockID]
	return leaf, ok
}

// Set assigns blockID to leaf.
func (p *InMemoryPositionMap) Set(blockID int, leaf int) {
	p.m[blockID] = leaf
}

// Size returns the number of blocks with assigned positions.
func (p *InMemoryPositionMap) Size() int {
	return len(p.m)
}
