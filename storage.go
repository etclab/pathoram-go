package pathoram

// Storage provides block-level access to the ORAM tree structure.
// Implementations may store data in memory, files, or remote services.
type Storage interface {
	// ReadBucket returns all blocks in the bucket at the given index.
	ReadBucket(idx int) ([]Block, error)

	// WriteBucket writes all blocks to the bucket at the given index.
	WriteBucket(idx int, blocks []Block) error

	// NumBuckets returns the total number of buckets in storage.
	NumBuckets() int

	// BucketSize returns the number of block slots per bucket.
	BucketSize() int

	// BlockSize returns the size of each block's data in bytes.
	BlockSize() int
}

// Block represents a single data block in storage.
// For encrypted storage, Data contains ciphertext.
type Block struct {
	ID   int    // Block ID (-1 = empty/dummy)
	Leaf int    // Assigned leaf position
	Data []byte // Block data (plaintext or ciphertext depending on encryptor)
}

// InMemoryStorage implements Storage using in-memory slices.
type InMemoryStorage struct {
	buckets    [][]Block
	bucketSize int
	blockSize  int
}

// NewInMemoryStorage creates a new in-memory storage with the given dimensions.
// All blocks are initialized as empty (ID = EmptyBlockID).
func NewInMemoryStorage(numBuckets, bucketSize, blockSize int) *InMemoryStorage {
	buckets := make([][]Block, numBuckets)
	for i := range buckets {
		buckets[i] = make([]Block, bucketSize)
		for j := range buckets[i] {
			buckets[i][j] = Block{
				ID:   EmptyBlockID,
				Leaf: -1,
				Data: make([]byte, blockSize),
			}
		}
	}
	return &InMemoryStorage{
		buckets:    buckets,
		bucketSize: bucketSize,
		blockSize:  blockSize,
	}
}

// ReadBucket returns a copy of all blocks in the bucket at idx.
func (s *InMemoryStorage) ReadBucket(idx int) ([]Block, error) {
	if idx < 0 || idx >= len(s.buckets) {
		return nil, ErrInvalidConfig
	}
	// Return a copy to prevent external modification
	result := make([]Block, len(s.buckets[idx]))
	for i, b := range s.buckets[idx] {
		result[i] = Block{
			ID:   b.ID,
			Leaf: b.Leaf,
			Data: make([]byte, len(b.Data)),
		}
		copy(result[i].Data, b.Data)
	}
	return result, nil
}

// WriteBucket writes all blocks to the bucket at idx.
func (s *InMemoryStorage) WriteBucket(idx int, blocks []Block) error {
	if idx < 0 || idx >= len(s.buckets) {
		return ErrInvalidConfig
	}
	if len(blocks) != s.bucketSize {
		return ErrInvalidConfig
	}
	for i, b := range blocks {
		s.buckets[idx][i] = Block{
			ID:   b.ID,
			Leaf: b.Leaf,
			Data: make([]byte, len(b.Data)),
		}
		copy(s.buckets[idx][i].Data, b.Data)
	}
	return nil
}

// NumBuckets returns the total number of buckets.
func (s *InMemoryStorage) NumBuckets() int {
	return len(s.buckets)
}

// BucketSize returns slots per bucket.
func (s *InMemoryStorage) BucketSize() int {
	return s.bucketSize
}

// BlockSize returns bytes per block.
func (s *InMemoryStorage) BlockSize() int {
	return s.blockSize
}
