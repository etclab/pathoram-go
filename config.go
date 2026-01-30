package pathoram

import "errors"

// EmptyBlockID marks a block slot as empty/dummy.
const EmptyBlockID = -1

var (
	ErrInvalidConfig    = errors.New("invalid PathORAM configuration")
	ErrInvalidBlockID   = errors.New("invalid block ID")
	ErrInvalidDataSize  = errors.New("data size doesn't match block size")
	ErrStashOverflow    = errors.New("stash overflow")
	ErrEncryptionFailed = errors.New("block encryption failed")
	ErrDecryptionFailed = errors.New("block decryption failed")
)

// EvictionStrategy defines how blocks are evicted from stash to tree.
type EvictionStrategy int

const (
	// EvictLevelByLevel iterates levels from leaf to root, filling slots greedily.
	// This is the original/baseline strategy.
	EvictLevelByLevel EvictionStrategy = iota

	// EvictGreedyByDepth places each block at its deepest possible level first.
	// Reduces stash pressure by maximizing depth utilization.
	EvictGreedyByDepth

	// EvictDeterministicTwoPath evicts along two paths per access.
	// Reduces stash size variance (Path ORAM optimization).
	EvictDeterministicTwoPath
)

// Config holds PathORAM configuration parameters.
type Config struct {
	NumBlocks        int              // Total number of blocks to support (valid IDs: 0 to NumBlocks-1)
	BlockSize        int              // Size of each block in bytes
	BucketSize       int              // Number of blocks per bucket (Z parameter)
	StashLimit       int              // Maximum stash size before error
	EvictionStrategy EvictionStrategy // Eviction strategy to use
	ConstantTime     bool             // Enable constant-time operations for TEE deployments
}

// Validate checks the configuration for errors and applies defaults.
// Returns a copy of the config with defaults applied.
func (c Config) Validate() (Config, error) {
	if c.NumBlocks <= 0 || c.BlockSize <= 0 {
		return c, ErrInvalidConfig
	}
	if c.BucketSize == 0 {
		c.BucketSize = 5
	}
	if c.StashLimit == 0 {
		c.StashLimit = 100
	}
	return c, nil
}

// ComputeTreeParams calculates tree dimensions from config.
// Returns (height, numLeaves, totalBuckets).
func (c Config) ComputeTreeParams() (height, numLeaves, totalBuckets int) {
	numBuckets := (c.NumBlocks + c.BucketSize - 1) / c.BucketSize
	height = 1
	for (1<<height)-1 < numBuckets {
		height++
	}
	numLeaves = 1 << (height - 1)
	totalBuckets = (1 << height) - 1
	return
}