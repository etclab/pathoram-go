package pathoram

import (
	"crypto/rand"
	"errors"
	"math/big"
)

var (
	ErrInvalidConfig   = errors.New("invalid PathORAM configuration")
	ErrInvalidBlockID  = errors.New("invalid block ID")
	ErrInvalidDataSize = errors.New("data size doesn't match block size")
	ErrStashOverflow   = errors.New("stash overflow")
)

// OpType represents the type of ORAM operation.
type OpType int

const (
	OpRead OpType = iota
	OpWrite
)

// Config holds PathORAM configuration parameters.
type Config struct {
	NumBlocks  int // Total number of blocks to support
	BlockSize  int // Size of each block in bytes
	BucketSize int // Number of blocks per bucket (Z parameter)
	StashLimit int // Maximum stash size before error
}

// block represents a single data block.
type block struct {
	id   int    // Block ID (-1 means dummy/empty)
	leaf int    // Assigned leaf position
	data []byte // Block data
}

// PathORAM implements the Path ORAM protocol.
type PathORAM struct {
	cfg       Config
	height    int
	numLeaves int

	posMap map[int]int // block ID -> leaf position
	stash  []block     // blocks not yet written back to tree
	tree   [][]block   // tree[bucketIdx] = slice of blocks in bucket
}

// NewPathORAM creates a new PathORAM instance with the given configuration.
func NewPathORAM(cfg Config) (*PathORAM, error) {
	if cfg.NumBlocks <= 0 || cfg.BlockSize <= 0 {
		return nil, ErrInvalidConfig
	}
	if cfg.BucketSize == 0 {
		cfg.BucketSize = 5
	}
	if cfg.StashLimit == 0 {
		cfg.StashLimit = 100
	}

	// Compute tree height: need enough buckets to hold all blocks
	numBuckets := (cfg.NumBlocks + cfg.BucketSize - 1) / cfg.BucketSize
	height := 1
	for (1<<height)-1 < numBuckets { // 2^h - 1 = total nodes in complete binary tree
		height++
	}
	numLeaves := 1 << (height - 1)            // 2^(h-1) leaves
	totalBuckets := (1 << height) - 1         // 2^h - 1 total nodes

	// Initialize tree with empty buckets
	tree := make([][]block, totalBuckets)
	for i := range tree {
		tree[i] = make([]block, cfg.BucketSize)
		for j := range tree[i] {
			tree[i][j] = block{id: -1, leaf: -1, data: make([]byte, cfg.BlockSize)}
		}
	}

	return &PathORAM{
		cfg:       cfg,
		height:    height,
		numLeaves: numLeaves,
		posMap:    make(map[int]int),
		stash:     nil,
		tree:      tree,
	}, nil
}

// Capacity returns the number of blocks this ORAM can store.
func (o *PathORAM) Capacity() int {
	return o.cfg.NumBlocks
}

// Height returns the height of the binary tree.
func (o *PathORAM) Height() int {
	return o.height
}

// NumLeaves returns the number of leaf nodes in the tree.
func (o *PathORAM) NumLeaves() int {
	return o.numLeaves
}

// StashSize returns the current number of blocks in the stash.
func (o *PathORAM) StashSize() int {
	return len(o.stash)
}

// Size returns the number of allocated blocks.
func (o *PathORAM) Size() int {
	return len(o.posMap)
}

// Access performs an oblivious read or write operation.
// For OpRead: returns current data (zeros if block doesn't exist), data param ignored.
// For OpWrite: stores data, returns previous value.
func (o *PathORAM) Access(op OpType, blockID int, data []byte) ([]byte, error) {
	if blockID < 0 || blockID >= o.cfg.NumBlocks {
		return nil, ErrInvalidBlockID
	}
	if op == OpWrite && len(data) != o.cfg.BlockSize {
		return nil, ErrInvalidDataSize
	}
	if op == OpRead {
		return o.access(blockID, nil)
	}
	return o.access(blockID, data)
}

// Path returns bucket indices from leaf to root.
// Leaf index is 0-based among all leaves.
func (o *PathORAM) Path(leaf int) []int {
	path := make([]int, o.height)
	// Convert leaf index to bucket index: leaves start at index numLeaves-1
	bucket := o.numLeaves - 1 + leaf
	for i := 0; i < o.height; i++ {
		path[i] = bucket
		bucket = (bucket - 1) / 2 // parent
	}
	return path
}

// randomLeaf returns a cryptographically random leaf index.
func (o *PathORAM) randomLeaf() int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(o.numLeaves)))
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return int(n.Int64())
}

// Read reads the block with the given ID.
func (o *PathORAM) Read(blockID int) ([]byte, error) {
	if blockID < 0 || blockID >= o.cfg.NumBlocks {
		return nil, ErrInvalidBlockID
	}
	data, err := o.access(blockID, nil)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Write writes data to the block with the given ID.
func (o *PathORAM) Write(blockID int, data []byte) error {
	if blockID < 0 || blockID >= o.cfg.NumBlocks {
		return ErrInvalidBlockID
	}
	if len(data) != o.cfg.BlockSize {
		return ErrInvalidDataSize
	}
	_, err := o.access(blockID, data)
	return err
}

// access performs the core PathORAM access operation.
// If newData is nil, it's a read; otherwise it's a write.
func (o *PathORAM) access(blockID int, newData []byte) ([]byte, error) {
	// Step 1: Look up or assign leaf position
	leaf, exists := o.posMap[blockID]
	if !exists {
		leaf = o.randomLeaf()
	}

	// Step 2: Assign new random leaf for this block
	o.posMap[blockID] = o.randomLeaf()

	// Step 3: Read path into stash
	path := o.Path(leaf)
	for _, bucketIdx := range path {
		for i := range o.tree[bucketIdx] {
			b := &o.tree[bucketIdx][i]
			if b.id != -1 {
				o.stash = append(o.stash, *b)
				b.id = -1 // mark as empty
			}
		}
	}

	// Step 4: Find the requested block in stash
	var result []byte
	foundIdx := -1
	for i, b := range o.stash {
		if b.id == blockID {
			foundIdx = i
			result = make([]byte, o.cfg.BlockSize)
			copy(result, b.data)
			break
		}
	}

	// Step 5: Handle read/write
	if foundIdx == -1 {
		// Block not found - new block or first read
		result = make([]byte, o.cfg.BlockSize)
		if newData != nil {
			copy(result, newData)
		}
		// Add block to stash
		newBlock := block{
			id:   blockID,
			leaf: o.posMap[blockID],
			data: make([]byte, o.cfg.BlockSize),
		}
		if newData != nil {
			copy(newBlock.data, newData)
		}
		o.stash = append(o.stash, newBlock)
	} else {
		// Update existing block
		o.stash[foundIdx].leaf = o.posMap[blockID]
		if newData != nil {
			copy(o.stash[foundIdx].data, newData)
			copy(result, newData)
		}
	}

	// Step 6: Eviction - write blocks back to path
	if err := o.evict(path); err != nil {
		return nil, err
	}

	return result, nil
}

// evict writes blocks from stash back to the path.
func (o *PathORAM) evict(path []int) error {
	// For each level from leaf to root, try to place blocks
	for level := 0; level < len(path); level++ {
		bucketIdx := path[level]
		bucket := o.tree[bucketIdx]

		// Find blocks in stash that can go to this bucket
		for slot := 0; slot < o.cfg.BucketSize; slot++ {
			if bucket[slot].id != -1 {
				continue // slot occupied
			}
			// Find a block whose path contains this bucket
			for i := 0; i < len(o.stash); i++ {
				b := &o.stash[i]
				if o.canPlaceAt(b.leaf, bucketIdx) {
					bucket[slot] = *b
					// Remove from stash
					o.stash = append(o.stash[:i], o.stash[i+1:]...)
					break
				}
			}
		}
	}

	// Check stash overflow
	if len(o.stash) > o.cfg.StashLimit {
		return ErrStashOverflow
	}
	return nil
}

// canPlaceAt returns true if a block assigned to the given leaf
// can be placed in the bucket at bucketIdx.
func (o *PathORAM) canPlaceAt(leaf, bucketIdx int) bool {
	path := o.Path(leaf)
	for _, idx := range path {
		if idx == bucketIdx {
			return true
		}
	}
	return false
}
