package pathoram

import (
	"bytes"
	"fmt"
	"testing"
)

// Constructor tests - table-driven
func TestNewPathORAM(t *testing.T) {
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
			oram, err := NewPathORAM(tt.cfg)
			if err != tt.wantErr {
				t.Errorf("NewPathORAM() error = %v, want %v", err, tt.wantErr)
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

func TestNewPathORAM_Defaults(t *testing.T) {
	t.Run("default bucket size", func(t *testing.T) {
		cfg := Config{NumBlocks: 100, BlockSize: 512}
		oram, err := NewPathORAM(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if oram.cfg.BucketSize != 5 {
			t.Errorf("BucketSize = %d, want default 5", oram.cfg.BucketSize)
		}
	})

	t.Run("default stash limit", func(t *testing.T) {
		cfg := Config{NumBlocks: 100, BlockSize: 512}
		oram, err := NewPathORAM(cfg)
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
			oram, _ := NewPathORAM(cfg)
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
			oram, _ := NewPathORAM(cfg)
			if got := oram.NumLeaves(); got != tt.wantLeaves {
				t.Errorf("NumLeaves() = %d, want %d", got, tt.wantLeaves)
			}
		})
	}
}

func TestTreeStructure(t *testing.T) {
	cfg := Config{NumBlocks: 16, BlockSize: 64, BucketSize: 4}
	oram, err := NewPathORAM(cfg)
	if err != nil {
		t.Fatalf("NewPathORAM() error = %v", err)
	}

	// Verify tree has correct number of buckets: 2^h - 1
	expectedBuckets := (1 << oram.Height()) - 1
	if len(oram.tree) != expectedBuckets {
		t.Errorf("tree has %d buckets, want %d", len(oram.tree), expectedBuckets)
	}

	// Verify each bucket has correct size
	for i, bucket := range oram.tree {
		if len(bucket) != cfg.BucketSize {
			t.Errorf("bucket %d has %d slots, want %d", i, len(bucket), cfg.BucketSize)
		}
	}

	// Verify all blocks are initially empty (id = -1)
	for i, bucket := range oram.tree {
		for j, block := range bucket {
			if block.id != EmptyBlockID {
				t.Errorf("bucket %d slot %d has id %d, want %d", i, j, block.id, EmptyBlockID)
			}
		}
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
	oram, _ := NewPathORAM(cfg)

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
	oram, _ := NewPathORAM(cfg)

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
	oram, _ := NewPathORAM(cfg)

	data := bytes.Repeat([]byte{0xAB}, 32)
	if err := oram.Write(0, data); err != nil {
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
	oram, _ := NewPathORAM(cfg)

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
	oram, _ := NewPathORAM(cfg)

	// Write to multiple blocks
	for i := 0; i < 10; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 16)
		if err := oram.Write(i, data); err != nil {
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
	oram, _ := NewPathORAM(cfg)

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
	oram, _ := NewPathORAM(cfg)

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
			err := oram.Write(tt.blockID, make([]byte, 16))
			if err != ErrInvalidBlockID {
				t.Errorf("Write(%d) error = %v, want ErrInvalidBlockID", tt.blockID, err)
			}
		})
	}
}

func TestAccess_WrongDataSize(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

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
			err := oram.Write(0, make([]byte, tt.size))
			if err != ErrInvalidDataSize {
				t.Errorf("Write with size %d error = %v, want ErrInvalidDataSize", tt.size, err)
			}
		})
	}
}

func TestAccess_UnifiedAPI(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

	data := bytes.Repeat([]byte{0xCC}, 16)
	_, err := oram.Access(OpWrite, 5, data)
	if err != nil {
		t.Fatalf("Access(OpWrite) failed: %v", err)
	}

	got, err := oram.Access(OpRead, 5, nil)
	if err != nil {
		t.Fatalf("Access(OpRead) failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Access(OpRead) = %x, want %x", got, data)
	}
}

func TestAccess_WriteReturnsPreviousValue(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

	// First write to new block should return zeros
	old, err := oram.Access(OpWrite, 0, bytes.Repeat([]byte{0xAA}, 16))
	if err != nil {
		t.Fatalf("first Access(OpWrite) failed: %v", err)
	}
	if !bytes.Equal(old, make([]byte, 16)) {
		t.Errorf("first write should return zeros, got %x", old)
	}

	// Second write should return previous value
	old, err = oram.Access(OpWrite, 0, bytes.Repeat([]byte{0xBB}, 16))
	if err != nil {
		t.Fatalf("second Access(OpWrite) failed: %v", err)
	}
	if !bytes.Equal(old, bytes.Repeat([]byte{0xAA}, 16)) {
		t.Errorf("second write should return 0xAA..., got %x", old)
	}

	// Third write should return second value
	old, err = oram.Access(OpWrite, 0, bytes.Repeat([]byte{0xCC}, 16))
	if err != nil {
		t.Fatalf("third Access(OpWrite) failed: %v", err)
	}
	if !bytes.Equal(old, bytes.Repeat([]byte{0xBB}, 16)) {
		t.Errorf("third write should return 0xBB..., got %x", old)
	}
}

// State tracking tests
func TestSize(t *testing.T) {
	cfg := Config{NumBlocks: 20, BlockSize: 16, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

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
	oram, _ := NewPathORAM(cfg)

	for i := 0; i < 50; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 32)
		if err := oram.Write(i, data); err != nil {
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
	oram, _ := NewPathORAM(cfg)

	// Write all blocks
	expected := make(map[int][]byte)
	for i := 0; i < 100; i++ {
		data := make([]byte, 64)
		for j := range data {
			data[j] = byte((i*7 + j) % 256)
		}
		expected[i] = data
		if err := oram.Write(i, data); err != nil {
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
			if err := oram.Write(blockID, newData); err != nil {
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
			oram, err := NewPathORAM(cfg)
			if err != nil {
				t.Fatalf("NewPathORAM failed: %v", err)
			}

			// Write all blocks with unique data
			expected := make(map[int][]byte)
			for i := 0; i < 64; i++ {
				data := bytes.Repeat([]byte{byte(i)}, 32)
				expected[i] = data
				if err := oram.Write(i, data); err != nil {
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
			oram, err := NewPathORAM(cfg)
			if err != nil {
				t.Fatalf("NewPathORAM failed: %v", err)
			}

			data := make([]byte, 16)
			maxStash := 0

			// Heavy write workload
			for i := 0; i < 128; i++ {
				if err := oram.Write(i, data); err != nil {
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
			oram, _ := NewPathORAM(cfg)

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

// Benchmarks
func BenchmarkAccess(b *testing.B) {
	numBlocksValues := []int{64, 256, 1024, 4096, 16384}
	blockSizes := []int{256, 1024, 4096}

	for _, numBlocks := range numBlocksValues {
		for _, blockSize := range blockSizes {
			cfg := Config{NumBlocks: numBlocks, BlockSize: blockSize}
			oram, _ := NewPathORAM(cfg)
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
		oram, _ := NewPathORAM(cfg)
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
		oram, _ := NewPathORAM(cfg)
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
			oram, err := NewPathORAM(cfg)
			if err != nil {
				b.Fatalf("NewPathORAM failed: %v", err)
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
		oram, _ := NewPathORAM(cfg)
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
