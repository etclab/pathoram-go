package pathoram

import (
	"bytes"
	"testing"
)

func TestNewPathORAM_ValidConfig(t *testing.T) {
	cfg := Config{
		NumBlocks:  100,
		BlockSize:  512,
		BucketSize: 5,
		StashLimit: 100,
	}
	oram, err := NewPathORAM(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oram == nil {
		t.Fatal("expected non-nil ORAM")
	}
	if oram.Capacity() != 100 {
		t.Errorf("Capacity() = %d, want 100", oram.Capacity())
	}
}

func TestNewPathORAM_InvalidConfig_ZeroBlocks(t *testing.T) {
	cfg := Config{
		NumBlocks:  0,
		BlockSize:  512,
		BucketSize: 5,
		StashLimit: 100,
	}
	_, err := NewPathORAM(cfg)
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestNewPathORAM_InvalidConfig_ZeroBlockSize(t *testing.T) {
	cfg := Config{
		NumBlocks:  100,
		BlockSize:  0,
		BucketSize: 5,
		StashLimit: 100,
	}
	_, err := NewPathORAM(cfg)
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestNewPathORAM_DefaultBucketSize(t *testing.T) {
	cfg := Config{
		NumBlocks: 100,
		BlockSize: 512,
		// BucketSize omitted - should default to 5
	}
	oram, err := NewPathORAM(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oram.cfg.BucketSize != 5 {
		t.Errorf("BucketSize = %d, want default 5", oram.cfg.BucketSize)
	}
}

func TestNewPathORAM_DefaultStashLimit(t *testing.T) {
	cfg := Config{
		NumBlocks: 100,
		BlockSize: 512,
		// StashLimit omitted - should default to 100
	}
	oram, err := NewPathORAM(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oram.cfg.StashLimit != 100 {
		t.Errorf("StashLimit = %d, want default 100", oram.cfg.StashLimit)
	}
}

// Tree structure tests
func TestTreeHeight(t *testing.T) {
	tests := []struct {
		numBlocks  int
		bucketSize int
		wantHeight int
	}{
		{1, 1, 1},      // 1 block, 1 per bucket = height 1
		{7, 1, 3},      // 7 blocks needs 7 buckets = height 3 (2^3-1=7)
		{8, 1, 4},      // 8 blocks needs 8 buckets = height 4
		{100, 5, 5},    // 100 blocks, 5 per bucket = 20 buckets, height 5
		{1000, 4, 8},   // 1000 blocks, 4 per bucket = 250 buckets, height 8
	}
	for _, tt := range tests {
		cfg := Config{NumBlocks: tt.numBlocks, BlockSize: 512, BucketSize: tt.bucketSize}
		oram, _ := NewPathORAM(cfg)
		if got := oram.Height(); got != tt.wantHeight {
			t.Errorf("Height() with numBlocks=%d, bucketSize=%d = %d, want %d",
				tt.numBlocks, tt.bucketSize, got, tt.wantHeight)
		}
	}
}

func TestNumLeaves(t *testing.T) {
	tests := []struct {
		numBlocks  int
		bucketSize int
		wantLeaves int
	}{
		{7, 1, 4},      // height 3 tree has 4 leaves
		{100, 5, 16},   // height 5 tree has 16 leaves
	}
	for _, tt := range tests {
		cfg := Config{NumBlocks: tt.numBlocks, BlockSize: 512, BucketSize: tt.bucketSize}
		oram, _ := NewPathORAM(cfg)
		if got := oram.NumLeaves(); got != tt.wantLeaves {
			t.Errorf("NumLeaves() with numBlocks=%d, bucketSize=%d = %d, want %d",
				tt.numBlocks, tt.bucketSize, got, tt.wantLeaves)
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
		got := oram.Path(tt.leaf)
		if len(got) != len(tt.wantPath) {
			t.Errorf("Path(%d) = %v, want %v", tt.leaf, got, tt.wantPath)
			continue
		}
		for i := range got {
			if got[i] != tt.wantPath[i] {
				t.Errorf("Path(%d) = %v, want %v", tt.leaf, got, tt.wantPath)
				break
			}
		}
	}
}

// Access operation tests
func TestAccess_WriteAndRead(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 32, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

	// Write data to block 0
	data := bytes.Repeat([]byte{0xAB}, 32)
	err := oram.Write(0, data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read it back
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

	// Read unwritten block should return zeros
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

	// Read all back and verify
	for i := 0; i < 10; i++ {
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

	// Write initial
	oram.Write(3, bytes.Repeat([]byte{0x11}, 16))

	// Overwrite
	newData := bytes.Repeat([]byte{0x22}, 16)
	oram.Write(3, newData)

	// Verify overwrite
	got, _ := oram.Read(3)
	if !bytes.Equal(got, newData) {
		t.Errorf("After overwrite: got %x, want %x", got, newData)
	}
}

func TestAccess_InvalidBlockID(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

	_, err := oram.Read(-1)
	if err != ErrInvalidBlockID {
		t.Errorf("Read(-1) error = %v, want ErrInvalidBlockID", err)
	}

	_, err = oram.Read(10)
	if err != ErrInvalidBlockID {
		t.Errorf("Read(10) error = %v, want ErrInvalidBlockID", err)
	}

	err = oram.Write(-1, make([]byte, 16))
	if err != ErrInvalidBlockID {
		t.Errorf("Write(-1) error = %v, want ErrInvalidBlockID", err)
	}
}

func TestAccess_WrongDataSize(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

	err := oram.Write(0, make([]byte, 8)) // too short
	if err != ErrInvalidDataSize {
		t.Errorf("Write with wrong size error = %v, want ErrInvalidDataSize", err)
	}
}

func TestAccess_StressTest(t *testing.T) {
	// Larger scale test to verify correctness under heavy load
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

		// Occasionally write
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

func TestStashSize(t *testing.T) {
	cfg := Config{NumBlocks: 50, BlockSize: 32, BucketSize: 4, StashLimit: 100}
	oram, _ := NewPathORAM(cfg)

	// Write some blocks and check stash doesn't grow unbounded
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

// Unified Access API tests
func TestAccess_UnifiedAPI(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 4}
	oram, _ := NewPathORAM(cfg)

	// Write using Access
	data := bytes.Repeat([]byte{0xCC}, 16)
	_, err := oram.Access(OpWrite, 5, data)
	if err != nil {
		t.Fatalf("Access(Write) failed: %v", err)
	}

	// Read using Access
	got, err := oram.Access(OpRead, 5, nil)
	if err != nil {
		t.Fatalf("Access(Read) failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Access(Read) = %x, want %x", got, data)
	}
}

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
}