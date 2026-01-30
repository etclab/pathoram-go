package pathoram

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
)

// Constructor tests - table-driven
func TestNewInMemory(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     Config{NumBlocks: 100, BlockSize: 512, BucketSize: 5, StashLimit: 100},
			wantErr: nil,
		},
		{
			name:    "zero blocks",
			cfg:     Config{NumBlocks: 0, BlockSize: 512, BucketSize: 5},
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "negative blocks",
			cfg:     Config{NumBlocks: -1, BlockSize: 512},
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "zero block size",
			cfg:     Config{NumBlocks: 100, BlockSize: 0, BucketSize: 5},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oram, err := NewInMemory(tt.cfg)
			if err != tt.wantErr {
				t.Errorf("NewInMemory() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if oram == nil {
					t.Fatal("expected non-nil ORAM")
				}
				if oram.Capacity() != tt.cfg.NumBlocks {
					t.Errorf("Capacity() = %d, want %d", oram.Capacity(), tt.cfg.NumBlocks)
				}
			}
		})
	}
}

func TestNewInMemory_Defaults(t *testing.T) {
	t.Run("default bucket size", func(t *testing.T) {
		cfg := Config{NumBlocks: 100, BlockSize: 512}
		oram, err := NewInMemory(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if oram.cfg.BucketSize != 5 {
			t.Errorf("BucketSize = %d, want default 5", oram.cfg.BucketSize)
		}
	})

	t.Run("default stash limit", func(t *testing.T) {
		cfg := Config{NumBlocks: 100, BlockSize: 512}
		oram, err := NewInMemory(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if oram.cfg.StashLimit != 100 {
			t.Errorf("StashLimit = %d, want default 100", oram.cfg.StashLimit)
		}
	})
}

// Tree structure tests
func TestTreeHeight(t *testing.T) {
	tests := []struct {
		numBlocks  int
		bucketSize int
		wantHeight int
	}{
		{1, 1, 1},    // 1 block, 1 per bucket = height 1
		{7, 1, 3},    // 7 blocks needs 7 buckets = height 3 (2^3-1=7)
		{8, 1, 4},    // 8 blocks needs 8 buckets = height 4
		{100, 5, 5},  // 100 blocks, 5 per bucket = 20 buckets, height 5
		{1000, 4, 8}, // 1000 blocks, 4 per bucket = 250 buckets, height 8
	}
	for _, tt := range tests {
		name := fmt.Sprintf("blocks=%d/Z=%d", tt.numBlocks, tt.bucketSize)
		t.Run(name, func(t *testing.T) {
			cfg := Config{NumBlocks: tt.numBlocks, BlockSize: 512, BucketSize: tt.bucketSize}
			oram, _ := NewInMemory(cfg)
			if got := oram.Height(); got != tt.wantHeight {
				t.Errorf("Height() = %d, want %d", got, tt.wantHeight)
			}
		})
	}
}

func TestNumLeaves(t *testing.T) {
	tests := []struct {
		numBlocks  int
		bucketSize int
		wantLeaves int
	}{
		{7, 1, 4},    // height 3 tree has 4 leaves
		{100, 5, 16}, // height 5 tree has 16 leaves
	}
	for _, tt := range tests {
		name := fmt.Sprintf("blocks=%d/Z=%d", tt.numBlocks, tt.bucketSize)
		t.Run(name, func(t *testing.T) {
			cfg := Config{NumBlocks: tt.numBlocks, BlockSize: 512, BucketSize: tt.bucketSize}
			oram, _ := NewInMemory(cfg)
			if got := oram.NumLeaves(); got != tt.wantLeaves {
				t.Errorf("NumLeaves() = %d, want %d", got, tt.wantLeaves)
			}
		})
	}
}

func TestPath(t *testing.T) {
	// Tree with height 3: 7 buckets (indices 0-6)
	//        0
	//       / \
	//      1   2
	//     / \ / \
	//    3  4 5  6
	// Leaves are 3,4,5,6 (indices 0,1,2,3 in leaf numbering)
	cfg := Config{NumBlocks: 7, BlockSize: 512, BucketSize: 1}
	oram, _ := NewInMemory(cfg)

	tests := []struct {
		leaf     int
		wantPath []int // leaf to root
	}{
		{0, []int{3, 1, 0}}, // leaf index 0 -> bucket 3 -> bucket 1 -> root
		{1, []int{4, 1, 0}}, // leaf index 1 -> bucket 4 -> bucket 1 -> root
		{2, []int{5, 2, 0}}, // leaf index 2 -> bucket 5 -> bucket 2 -> root
		{3, []int{6, 2, 0}}, // leaf index 3 -> bucket 6 -> bucket 2 -> root
	}
	for _, tt := range tests {
		name := fmt.Sprintf("leaf=%d", tt.leaf)
		t.Run(name, func(t *testing.T) {
			got := oram.Path(tt.leaf)
			if len(got) != len(tt.wantPath) {
				t.Errorf("Path(%d) = %v, want %v", tt.leaf, got, tt.wantPath)
				return
			}
			for i := range got {
				if got[i] != tt.wantPath[i] {
					t.Errorf("Path(%d) = %v, want %v", tt.leaf, got, tt.wantPath)
					break
				}
			}
		})
	}
}

func TestCanPlaceAt(t *testing.T) {
	cfg := Config{NumBlocks: 16, BlockSize: 64, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	// A block assigned to leaf 0 should be placeable on its path
	path := oram.Path(0)
	for _, bucketIdx := range path {
		if !oram.canPlaceAt(0, bucketIdx) {
			t.Errorf("canPlaceAt(0, %d) = false, want true", bucketIdx)
		}
	}

	// Root (bucket 0) should be reachable from any leaf
	for leaf := 0; leaf < oram.NumLeaves(); leaf++ {
		if !oram.canPlaceAt(leaf, 0) {
			t.Errorf("canPlaceAt(%d, 0) = false, want true (root)", leaf)
		}
	}
}

// Access operation tests
func TestAccess_WriteAndRead(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 32, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	data := bytes.Repeat([]byte{0xAB}, 32)
	if _, err := oram.Write(0, data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := oram.Read(0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Read returned %x, want %x", got, data)
	}
}

func TestAccess_ReadUnwritten(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 32, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	got, err := oram.Read(5)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	expected := make([]byte, 32)
	if !bytes.Equal(got, expected) {
		t.Errorf("Read unwritten block returned %x, want zeros", got)
	}
}

func TestAccess_MultipleBlocks(t *testing.T) {
	cfg := Config{NumBlocks: 20, BlockSize: 16, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	// Write to multiple blocks
	for i := 0; i < 10; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 16)
		if _, err := oram.Write(i, data); err != nil {
			t.Fatalf("Write(%d) failed: %v", i, err)
		}
	}

	// Read all back in reverse order
	for i := 9; i >= 0; i-- {
		got, err := oram.Read(i)
		if err != nil {
			t.Fatalf("Read(%d) failed: %v", i, err)
		}
		expected := bytes.Repeat([]byte{byte(i)}, 16)
		if !bytes.Equal(got, expected) {
			t.Errorf("Read(%d) = %x, want %x", i, got, expected)
		}
	}
}

func TestAccess_Overwrite(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	oram.Write(3, bytes.Repeat([]byte{0x11}, 16))

	newData := bytes.Repeat([]byte{0x22}, 16)
	oram.Write(3, newData)

	got, _ := oram.Read(3)
	if !bytes.Equal(got, newData) {
		t.Errorf("After overwrite: got %x, want %x", got, newData)
	}
}

func TestAccess_InvalidBlockID(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	tests := []struct {
		name    string
		blockID int
	}{
		{"negative", -1},
		{"at capacity", 10},
		{"beyond capacity", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name+" read", func(t *testing.T) {
			_, err := oram.Read(tt.blockID)
			if err != ErrInvalidBlockID {
				t.Errorf("Read(%d) error = %v, want ErrInvalidBlockID", tt.blockID, err)
			}
		})
		t.Run(tt.name+" write", func(t *testing.T) {
			_, err := oram.Write(tt.blockID, make([]byte, 16))
			if err != ErrInvalidBlockID {
				t.Errorf("Write(%d) error = %v, want ErrInvalidBlockID", tt.blockID, err)
			}
		})
	}
}

func TestAccess_WrongDataSize(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	tests := []struct {
		name string
		size int
	}{
		{"too short", 8},
		{"too long", 32},
		{"empty", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := oram.Write(0, make([]byte, tt.size))
			if err != ErrInvalidDataSize {
				t.Errorf("Write with size %d error = %v, want ErrInvalidDataSize", tt.size, err)
			}
		})
	}
}

func TestAccess_UnifiedAPI(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	data := bytes.Repeat([]byte{0xCC}, 16)
	_, err := oram.Access(5, data) // write
	if err != nil {
		t.Fatalf("Access(write) failed: %v", err)
	}

	got, err := oram.Access(5, nil) // read
	if err != nil {
		t.Fatalf("Access(read) failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Access(read) = %x, want %x", got, data)
	}
}

func TestAccess_WriteReturnsPreviousValue(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	// First write to new block should return zeros
	old, err := oram.Write(0, bytes.Repeat([]byte{0xAA}, 16))
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}
	if !bytes.Equal(old, make([]byte, 16)) {
		t.Errorf("first write should return zeros, got %x", old)
	}

	// Second write should return previous value
	old, err = oram.Write(0, bytes.Repeat([]byte{0xBB}, 16))
	if err != nil {
		t.Fatalf("second Write failed: %v", err)
	}
	if !bytes.Equal(old, bytes.Repeat([]byte{0xAA}, 16)) {
		t.Errorf("second write should return 0xAA..., got %x", old)
	}

	// Third write should return second value
	old, err = oram.Write(0, bytes.Repeat([]byte{0xCC}, 16))
	if err != nil {
		t.Fatalf("third Write failed: %v", err)
	}
	if !bytes.Equal(old, bytes.Repeat([]byte{0xBB}, 16)) {
		t.Errorf("third write should return 0xBB..., got %x", old)
	}
}

// State tracking tests
func TestSize(t *testing.T) {
	cfg := Config{NumBlocks: 20, BlockSize: 16, BucketSize: 4}
	oram, _ := NewInMemory(cfg)

	if oram.Size() != 0 {
		t.Errorf("Initial Size() = %d, want 0", oram.Size())
	}

	oram.Write(0, make([]byte, 16))
	oram.Write(5, make([]byte, 16))
	oram.Write(10, make([]byte, 16))

	if oram.Size() != 3 {
		t.Errorf("After 3 writes, Size() = %d, want 3", oram.Size())
	}

	// Re-write same block shouldn't increase size
	oram.Write(5, make([]byte, 16))
	if oram.Size() != 3 {
		t.Errorf("After re-write, Size() = %d, want 3", oram.Size())
	}

	// Read creates entry in posMap
	oram.Read(15)
	if oram.Size() != 4 {
		t.Errorf("After read of new block, Size() = %d, want 4", oram.Size())
	}
}

func TestStashSize(t *testing.T) {
	cfg := Config{NumBlocks: 50, BlockSize: 32, BucketSize: 4, StashLimit: 100}
	oram, _ := NewInMemory(cfg)

	for i := 0; i < 50; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 32)
		if _, err := oram.Write(i, data); err != nil {
			t.Fatalf("Write(%d) failed: %v", i, err)
		}
	}

	if oram.StashSize() > cfg.StashLimit {
		t.Errorf("StashSize() = %d, exceeds limit %d", oram.StashSize(), cfg.StashLimit)
	}
}

// Stress test
func TestAccess_StressTest(t *testing.T) {
	cfg := Config{NumBlocks: 100, BlockSize: 64, BucketSize: 4, StashLimit: 200}
	oram, _ := NewInMemory(cfg)

	// Write all blocks
	expected := make(map[int][]byte)
	for i := 0; i < 100; i++ {
		data := make([]byte, 64)
		for j := range data {
			data[j] = byte((i*7 + j) % 256)
		}
		expected[i] = data
		if _, err := oram.Write(i, data); err != nil {
			t.Fatalf("Write(%d) failed: %v", i, err)
		}
	}

	// Random read/write pattern
	for round := 0; round < 200; round++ {
		blockID := (round * 17) % 100
		got, err := oram.Read(blockID)
		if err != nil {
			t.Fatalf("Read(%d) round %d failed: %v", blockID, round, err)
		}
		if !bytes.Equal(got, expected[blockID]) {
			t.Errorf("Round %d: Read(%d) mismatch", round, blockID)
		}

		if round%3 == 0 {
			newData := make([]byte, 64)
			for j := range newData {
				newData[j] = byte((round + j) % 256)
			}
			expected[blockID] = newData
			if _, err := oram.Write(blockID, newData); err != nil {
				t.Fatalf("Write(%d) round %d failed: %v", blockID, round, err)
			}
		}
	}
}

// Eviction strategy unit tests
func TestEvictionStrategies_Correctness(t *testing.T) {
	strategies := []struct {
		name     string
		strategy EvictionStrategy
	}{
		{"LevelByLevel", EvictLevelByLevel},
		{"GreedyByDepth", EvictGreedyByDepth},
		{"DeterministicTwoPath", EvictDeterministicTwoPath},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			cfg := Config{
				NumBlocks:        64,
				BlockSize:        32,
				BucketSize:       4,
				StashLimit:       100,
				EvictionStrategy: s.strategy,
			}
			oram, err := NewInMemory(cfg)
			if err != nil {
				t.Fatalf("NewInMemory failed: %v", err)
			}

			// Write all blocks with unique data
			expected := make(map[int][]byte)
			for i := 0; i < 64; i++ {
				data := bytes.Repeat([]byte{byte(i)}, 32)
				expected[i] = data
				if _, err := oram.Write(i, data); err != nil {
					t.Fatalf("Write(%d) failed: %v", i, err)
				}
			}

			// Read all back and verify
			for i := 0; i < 64; i++ {
				got, err := oram.Read(i)
				if err != nil {
					t.Fatalf("Read(%d) failed: %v", i, err)
				}
				if !bytes.Equal(got, expected[i]) {
					t.Errorf("Read(%d) mismatch: got %x, want %x", i, got, expected[i])
				}
			}
		})
	}
}

func TestEvictionStrategies_StashBehavior(t *testing.T) {
	strategies := []struct {
		name     string
		strategy EvictionStrategy
	}{
		{"LevelByLevel", EvictLevelByLevel},
		{"GreedyByDepth", EvictGreedyByDepth},
		{"DeterministicTwoPath", EvictDeterministicTwoPath},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			cfg := Config{
				NumBlocks:        128,
				BlockSize:        16,
				BucketSize:       4,
				StashLimit:       200,
				EvictionStrategy: s.strategy,
			}
			oram, err := NewInMemory(cfg)
			if err != nil {
				t.Fatalf("NewInMemory failed: %v", err)
			}

			data := make([]byte, 16)
			maxStash := 0

			// Heavy write workload
			for i := 0; i < 128; i++ {
				if _, err := oram.Write(i, data); err != nil {
					t.Fatalf("Write(%d) failed: %v", i, err)
				}
				if oram.StashSize() > maxStash {
					maxStash = oram.StashSize()
				}
			}

			// Random access pattern
			for round := 0; round < 500; round++ {
				blockID := (round * 13) % 128
				if _, err := oram.Read(blockID); err != nil {
					t.Fatalf("Read(%d) round %d failed: %v", blockID, round, err)
				}
				if oram.StashSize() > maxStash {
					maxStash = oram.StashSize()
				}
			}

			t.Logf("%s: maxStash=%d, finalStash=%d", s.name, maxStash, oram.StashSize())

			// Stash should not exceed limit
			if oram.StashSize() > cfg.StashLimit {
				t.Errorf("Stash overflow: %d > %d", oram.StashSize(), cfg.StashLimit)
			}
		})
	}
}

func TestEvictionStrategies_Overwrite(t *testing.T) {
	strategies := []struct {
		name     string
		strategy EvictionStrategy
	}{
		{"LevelByLevel", EvictLevelByLevel},
		{"GreedyByDepth", EvictGreedyByDepth},
		{"DeterministicTwoPath", EvictDeterministicTwoPath},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			cfg := Config{
				NumBlocks:        32,
				BlockSize:        16,
				BucketSize:       4,
				EvictionStrategy: s.strategy,
			}
			oram, _ := NewInMemory(cfg)

			// Write, overwrite, verify pattern
			for round := 0; round < 10; round++ {
				data := bytes.Repeat([]byte{byte(round)}, 16)
				for i := 0; i < 32; i++ {
					oram.Write(i, data)
				}

				// Verify all have latest value
				for i := 0; i < 32; i++ {
					got, _ := oram.Read(i)
					if !bytes.Equal(got, data) {
						t.Errorf("Round %d, block %d: got %x, want %x", round, i, got, data)
					}
				}
			}
		})
	}
}

// Interface tests
func TestInMemoryStorage(t *testing.T) {
	storage := NewInMemoryStorage(7, 4, 64)

	if storage.NumBuckets() != 7 {
		t.Errorf("NumBuckets() = %d, want 7", storage.NumBuckets())
	}
	if storage.BucketSize() != 4 {
		t.Errorf("BucketSize() = %d, want 4", storage.BucketSize())
	}
	if storage.BlockSize() != 64 {
		t.Errorf("BlockSize() = %d, want 64", storage.BlockSize())
	}

	// Read initial bucket - should be empty
	bucket, err := storage.ReadBucket(0)
	if err != nil {
		t.Fatalf("ReadBucket failed: %v", err)
	}
	for i, b := range bucket {
		if b.ID != EmptyBlockID {
			t.Errorf("bucket[%d].ID = %d, want %d", i, b.ID, EmptyBlockID)
		}
	}

	// Write and read back
	testBlocks := []Block{
		{ID: 1, Leaf: 0, Data: bytes.Repeat([]byte{0x11}, 64)},
		{ID: 2, Leaf: 1, Data: bytes.Repeat([]byte{0x22}, 64)},
		{ID: EmptyBlockID, Leaf: -1, Data: make([]byte, 64)},
		{ID: EmptyBlockID, Leaf: -1, Data: make([]byte, 64)},
	}
	if err := storage.WriteBucket(0, testBlocks); err != nil {
		t.Fatalf("WriteBucket failed: %v", err)
	}

	bucket, _ = storage.ReadBucket(0)
	if bucket[0].ID != 1 || bucket[1].ID != 2 {
		t.Errorf("bucket contents mismatch after write")
	}
	if !bytes.Equal(bucket[0].Data, bytes.Repeat([]byte{0x11}, 64)) {
		t.Errorf("bucket[0].Data mismatch")
	}
}

func TestInMemoryPositionMap(t *testing.T) {
	posMap := NewInMemoryPositionMap()

	if posMap.Size() != 0 {
		t.Errorf("Initial Size() = %d, want 0", posMap.Size())
	}

	// Get non-existent
	_, exists := posMap.Get(5)
	if exists {
		t.Error("Get(5) should return exists=false for empty map")
	}

	// Set and get
	posMap.Set(5, 10)
	leaf, exists := posMap.Get(5)
	if !exists || leaf != 10 {
		t.Errorf("Get(5) = (%d, %v), want (10, true)", leaf, exists)
	}

	if posMap.Size() != 1 {
		t.Errorf("Size() = %d, want 1", posMap.Size())
	}

	// Update
	posMap.Set(5, 20)
	leaf, _ = posMap.Get(5)
	if leaf != 20 {
		t.Errorf("After update, Get(5) = %d, want 20", leaf)
	}
}

func TestAESGCMEncryptor(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	enc, err := NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("NewAESGCMEncryptor failed: %v", err)
	}

	plaintext := []byte("hello world 1234") // 16 bytes

	// Encrypt
	ciphertext, err := enc.Encrypt(1, 2, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Ciphertext should be longer due to nonce + tag
	if len(ciphertext) != len(plaintext)+enc.Overhead() {
		t.Errorf("ciphertext length = %d, want %d", len(ciphertext), len(plaintext)+enc.Overhead())
	}

	// Decrypt
	decrypted, err := enc.Decrypt(1, 2, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypt mismatch: got %x, want %x", decrypted, plaintext)
	}

	// Wrong blockID should fail
	_, err = enc.Decrypt(999, 2, ciphertext)
	if err != ErrDecryptionFailed {
		t.Errorf("Decrypt with wrong blockID should fail, got %v", err)
	}

	// Each encryption should produce different ciphertext (random nonce)
	ct1, _ := enc.Encrypt(1, 2, plaintext)
	ct2, _ := enc.Encrypt(1, 2, plaintext)
	if bytes.Equal(ct1, ct2) {
		t.Error("Two encryptions of same plaintext should differ (random nonce)")
	}
}

func TestNoOpEncryptor(t *testing.T) {
	enc := NoOpEncryptor{}

	plaintext := []byte("test data")

	ct, err := enc.Encrypt(1, 2, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if !bytes.Equal(ct, plaintext) {
		t.Error("NoOpEncryptor should return plaintext unchanged")
	}

	pt, err := enc.Decrypt(1, 2, ct)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Error("NoOpEncryptor Decrypt should return input unchanged")
	}

	if enc.Overhead() != 0 {
		t.Errorf("Overhead() = %d, want 0", enc.Overhead())
	}
}

func TestNew_WithExplicitParams(t *testing.T) {
	cfg := Config{NumBlocks: 64, BlockSize: 32, BucketSize: 4}
	cfg, _ = cfg.Validate()
	_, _, totalBuckets := cfg.ComputeTreeParams()

	storage := NewInMemoryStorage(totalBuckets, cfg.BucketSize, cfg.BlockSize)
	posMap := NewInMemoryPositionMap()
	enc := NoOpEncryptor{}

	oram, err := New(cfg, storage, posMap, enc)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Basic functionality test
	data := bytes.Repeat([]byte{0xAB}, 32)
	if _, err := oram.Write(0, data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := oram.Read(0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Read mismatch")
	}
}

func TestWithEncryption(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	cfg := Config{NumBlocks: 64, BlockSize: 32, BucketSize: 4}
	cfg, _ = cfg.Validate()
	_, _, totalBuckets := cfg.ComputeTreeParams()

	storage := NewInMemoryStorage(totalBuckets, cfg.BucketSize, cfg.BlockSize+28) // +28 for nonce+tag
	posMap := NewInMemoryPositionMap()
	enc, _ := NewAESGCMEncryptor(key)

	oram, err := New(cfg, storage, posMap, enc)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Write and read back
	data := make([]byte, 32)
	copy(data, []byte("secret test data"))
	if _, err := oram.Write(0, data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := oram.Read(0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Read mismatch: got %x, want %x", got, data)
	}

	// Verify storage contains encrypted (not plaintext) data
	// After write, block should be evicted to storage
	for i := 0; i < storage.NumBuckets(); i++ {
		bucket, _ := storage.ReadBucket(i)
		for _, b := range bucket {
			if b.ID != EmptyBlockID && bytes.Contains(b.Data, []byte("secret")) {
				t.Error("storage contains plaintext - encryption not working!")
			}
		}
	}
}

func TestConstantTimeMode(t *testing.T) {
	cfg := Config{
		NumBlocks:    64,
		BlockSize:    32,
		BucketSize:   4,
		ConstantTime: true,
	}
	oram, err := NewInMemory(cfg)
	if err != nil {
		t.Fatalf("NewInMemory failed: %v", err)
	}

	// Basic correctness - constant time mode should still work correctly
	expected := make(map[int][]byte)
	for i := 0; i < 32; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 32)
		expected[i] = data
		if _, err := oram.Write(i, data); err != nil {
			t.Fatalf("Write(%d) failed: %v", i, err)
		}
	}

	for i := 0; i < 32; i++ {
		got, err := oram.Read(i)
		if err != nil {
			t.Fatalf("Read(%d) failed: %v", i, err)
		}
		if !bytes.Equal(got, expected[i]) {
			t.Errorf("Read(%d) mismatch", i)
		}
	}
}

// Benchmarks
func BenchmarkAccess(b *testing.B) {
	numBlocksValues := []int{64, 256, 1024, 4096, 16384}
	blockSizes := []int{256, 1024, 4096}

	for _, numBlocks := range numBlocksValues {
		for _, blockSize := range blockSizes {
			cfg := Config{NumBlocks: numBlocks, BlockSize: blockSize}
			oram, _ := NewInMemory(cfg)
			data := make([]byte, blockSize)

			name := fmt.Sprintf("blocks=%d/blockSize=%d", numBlocks, blockSize)
			b.Run(name+"/write", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					oram.Write(i%numBlocks, data)
				}
			})

			// Pre-populate for read benchmark
			for i := 0; i < numBlocks; i++ {
				oram.Write(i, data)
			}

			b.Run(name+"/read", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					oram.Read(i % numBlocks)
				}
			})
		}
	}
}

// Benchmark varying tree height
func BenchmarkByTreeHeight(b *testing.B) {
	for height := 2; height <= 10; height++ {
		numBuckets := (1 << height) - 1
		numBlocks := numBuckets * 4

		cfg := Config{NumBlocks: numBlocks, BlockSize: 1024, BucketSize: 4}
		oram, _ := NewInMemory(cfg)
		data := make([]byte, 1024)

		for i := 0; i < numBlocks; i++ {
			oram.Write(i, data)
		}

		name := fmt.Sprintf("height=%d/buckets=%d", height, numBuckets)
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				oram.Read(i % numBlocks)
			}
		})
	}
}

// Benchmark varying bucket size (Z parameter)
func BenchmarkByBucketSize(b *testing.B) {
	bucketSizes := []int{2, 4, 6, 8, 10}
	numBlocks := 1024

	for _, z := range bucketSizes {
		cfg := Config{NumBlocks: numBlocks, BlockSize: 1024, BucketSize: z}
		oram, _ := NewInMemory(cfg)
		data := make([]byte, 1024)

		for i := 0; i < numBlocks; i++ {
			oram.Write(i, data)
		}

		name := fmt.Sprintf("Z=%d/height=%d", z, oram.Height())
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				oram.Read(i % numBlocks)
			}
		})
	}
}

// Benchmark comparing eviction strategies
func BenchmarkEvictionStrategy(b *testing.B) {
	strategies := []struct {
		name     string
		strategy EvictionStrategy
	}{
		{"LevelByLevel", EvictLevelByLevel},
		{"GreedyByDepth", EvictGreedyByDepth},
		{"DeterministicTwoPath", EvictDeterministicTwoPath},
	}

	heights := []int{5, 7, 9}

	for _, h := range heights {
		numBuckets := (1 << h) - 1
		numBlocks := numBuckets * 4

		for _, s := range strategies {
			cfg := Config{
				NumBlocks:        numBlocks,
				BlockSize:        1024,
				BucketSize:       4,
				EvictionStrategy: s.strategy,
			}
			oram, err := NewInMemory(cfg)
			if err != nil {
				b.Fatalf("NewInMemory failed: %v", err)
			}
			data := make([]byte, 1024)

			for i := 0; i < numBlocks; i++ {
				oram.Write(i, data)
			}

			name := fmt.Sprintf("height=%d/%s", h, s.name)
			b.Run(name, func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					oram.Read(i % numBlocks)
				}
			})
		}
	}
}

// Benchmark stash size under different strategies
func BenchmarkStashSizeByStrategy(b *testing.B) {
	strategies := []struct {
		name     string
		strategy EvictionStrategy
	}{
		{"LevelByLevel", EvictLevelByLevel},
		{"GreedyByDepth", EvictGreedyByDepth},
		{"DeterministicTwoPath", EvictDeterministicTwoPath},
	}

	numBlocks := 1024

	for _, s := range strategies {
		cfg := Config{
			NumBlocks:        numBlocks,
			BlockSize:        256,
			BucketSize:       4,
			StashLimit:       500,
			EvictionStrategy: s.strategy,
		}
		oram, _ := NewInMemory(cfg)
		data := make([]byte, 256)

		b.Run(s.name, func(b *testing.B) {
			for i := 0; i < numBlocks; i++ {
				oram.Write(i, data)
			}

			b.ResetTimer()
			maxStash := 0
			for i := 0; i < b.N; i++ {
				oram.Read(i % numBlocks)
				if oram.StashSize() > maxStash {
					maxStash = oram.StashSize()
				}
			}
			b.ReportMetric(float64(maxStash), "max_stash")
			b.ReportMetric(float64(oram.StashSize()), "final_stash")
		})
	}
}
