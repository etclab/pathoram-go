package pathoram

import (
	"crypto/rand"
	"math/big"
)

// block represents a single data block (internal, plaintext).
type block struct {
	id   int    // Block ID (-1 = empty/dummy)
	leaf int    // Assigned leaf position
	data []byte // Block data (plaintext)
}

// PathORAM implements the Path ORAM protocol.
type PathORAM struct {
	cfg       Config
	height    int
	numLeaves int

	storage Storage     // pluggable storage backend
	posMap  PositionMap // pluggable position map
	encrypt Encryptor   // pluggable encryption

	stash []block // blocks not yet written back to tree
}

// New creates a new PathORAM instance with explicit dependencies.
// Use this constructor when you need custom storage, position map, or encryption.
func New(cfg Config, storage Storage, posMap PositionMap, enc Encryptor) (*PathORAM, error) {
	cfg, err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	height, numLeaves, _ := cfg.ComputeTreeParams()

	return &PathORAM{
		cfg:       cfg,
		height:    height,
		numLeaves: numLeaves,
		storage:   storage,
		posMap:    posMap,
		encrypt:   enc,
		stash:     nil,
	}, nil
}

// NewInMemory creates a new PathORAM instance with in-memory storage and no encryption.
// This is the simplest way to create a PathORAM for testing or in-memory use.
func NewInMemory(cfg Config) (*PathORAM, error) {
	cfg, err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	_, _, totalBuckets := cfg.ComputeTreeParams()

	storage := NewInMemoryStorage(totalBuckets, cfg.BucketSize, cfg.BlockSize)
	posMap := NewInMemoryPositionMap()
	enc := NoOpEncryptor{}

	return New(cfg, storage, posMap, enc)
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
	return o.posMap.Size()
}

// BlockSize returns the configured block size.
func (o *PathORAM) BlockSize() int {
	return o.cfg.BlockSize
}

// Access performs an oblivious read or write operation.
// Valid block IDs are 0 to NumBlocks-1.
// If newData is nil, performs a read and returns current data (zeros if block doesn't exist).
// If newData is non-nil, performs a write and returns previous value.
func (o *PathORAM) Access(blockID int, newData []byte) ([]byte, error) {
	if blockID < 0 || blockID >= o.cfg.NumBlocks {
		return nil, ErrInvalidBlockID
	}
	if newData != nil && len(newData) != o.cfg.BlockSize {
		return nil, ErrInvalidDataSize
	}
	return o.access(blockID, newData)
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
// Returns the previous value stored at this block.
func (o *PathORAM) Write(blockID int, data []byte) ([]byte, error) {
	if blockID < 0 || blockID >= o.cfg.NumBlocks {
		return nil, ErrInvalidBlockID
	}
	if len(data) != o.cfg.BlockSize {
		return nil, ErrInvalidDataSize
	}
	return o.access(blockID, data)
}

// randomLeaf returns a cryptographically random leaf index.
func (o *PathORAM) randomLeaf() int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(o.numLeaves)))
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return int(n.Int64())
}

// access performs the core PathORAM access operation.
// If newData is nil, it's a read; otherwise it's a write.
func (o *PathORAM) access(blockID int, newData []byte) ([]byte, error) {
	// Step 1: Look up or assign leaf position
	leaf, exists := o.posMap.Get(blockID)
	if !exists {
		leaf = o.randomLeaf()
	}

	// Step 2: Assign new random leaf for this block
	o.posMap.Set(blockID, o.randomLeaf())

	// Step 3: Read path into stash
	path := o.Path(leaf)
	if err := o.readPathIntoStash(path); err != nil {
		return nil, err
	}

	// Step 4: Find the requested block in stash
	var result []byte
	var foundIdx int
	if o.cfg.ConstantTime {
		foundIdx, result = o.findInStashConstantTime(blockID)
	} else {
		foundIdx, result = o.findInStash(blockID)
	}

	// Step 5: Handle read/write
	if foundIdx == -1 {
		// Block not found - new block or first read
		// Previous value is zeros (per Path ORAM spec)
		result = make([]byte, o.cfg.BlockSize)
		// Add block to stash
		newLeaf, _ := o.posMap.Get(blockID)
		newBlock := block{
			id:   blockID,
			leaf: newLeaf,
			data: make([]byte, o.cfg.BlockSize),
		}
		if newData != nil {
			copy(newBlock.data, newData)
		}
		o.stash = append(o.stash, newBlock)
	} else {
		// Update existing block
		newLeaf, _ := o.posMap.Get(blockID)
		o.stash[foundIdx].leaf = newLeaf
		if newData != nil {
			copy(o.stash[foundIdx].data, newData)
		}
	}

	// Step 6: Eviction - write blocks back to path
	var err error
	if o.cfg.ConstantTime {
		err = o.evictConstantTime(path)
	} else {
		err = o.evictWithStrategy(path)
	}
	if err != nil {
		return nil, err
	}

	return result, nil
}

// findInStash searches stash for blockID.
// Returns (index, data) where index is -1 if not found.
func (o *PathORAM) findInStash(blockID int) (int, []byte) {
	for i, b := range o.stash {
		if b.id == blockID {
			result := make([]byte, o.cfg.BlockSize)
			copy(result, b.data)
			return i, result
		}
	}
	return -1, nil
}

// readPathIntoStash reads all blocks from path into stash.
func (o *PathORAM) readPathIntoStash(path []int) error {
	for _, bucketIdx := range path {
		bucket, err := o.storage.ReadBucket(bucketIdx)
		if err != nil {
			return err
		}
		for i := range bucket {
			if bucket[i].ID != EmptyBlockID {
				// Decrypt block data
				plaintext, err := o.encrypt.Decrypt(bucket[i].ID, bucket[i].Leaf, bucket[i].Data)
				if err != nil {
					return err
				}
				o.stash = append(o.stash, block{
					id:   bucket[i].ID,
					leaf: bucket[i].Leaf,
					data: plaintext,
				})
				// Mark as empty in storage
				bucket[i].ID = EmptyBlockID
			}
		}
		if err := o.storage.WriteBucket(bucketIdx, bucket); err != nil {
			return err
		}
	}
	return nil
}

// blockToStorage converts internal block to storage Block with encryption.
func (o *PathORAM) blockToStorage(b block) Block {
	ciphertext, err := o.encrypt.Encrypt(b.id, b.leaf, b.data)
	if err != nil {
		// Encryption should not fail with valid data
		panic("encryption failed: " + err.Error())
	}
	return Block{
		ID:   b.id,
		Leaf: b.leaf,
		Data: ciphertext,
	}
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

// canPlaceAt returns true if a block assigned to the given leaf
// can be placed in the bucket at bucketIdx.
// Uses ancestry check: bucket B is on leaf L's path iff L's leaf bucket
// is in the subtree rooted at B.
func (o *PathORAM) canPlaceAt(leaf, bucketIdx int) bool {
	// Leaf's bucket index
	leafBucket := o.numLeaves - 1 + leaf

	// Walk from leafBucket to root, checking if we hit bucketIdx
	for b := leafBucket; b >= 0; b = (b - 1) / 2 {
		if b == bucketIdx {
			return true
		}
		if b == 0 {
			break
		}
	}
	return false
}
