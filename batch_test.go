package pathoram

import (
	"bytes"
	"fmt"
	"testing"
)

func TestWriteBatch_Correctness(t *testing.T) {
	n := 200
	blockSize := 32
	cfg := Config{NumBlocks: n, BlockSize: blockSize, BucketSize: 5, StashLimit: n + 100}
	oram, err := NewInMemory(cfg)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	// Build batch
	items := make([]BatchItem, n)
	expected := make(map[int][]byte)
	for i := range n {
		data := bytes.Repeat([]byte{byte(i)}, blockSize)
		items[i] = BatchItem{BlockID: i, Data: data}
		expected[i] = data
	}

	if err := oram.WriteBatch(items); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	// Read back every block sequentially and verify
	for i := range n {
		got, err := oram.Read(i)
		if err != nil {
			t.Fatalf("Read(%d): %v", i, err)
		}
		if !bytes.Equal(got, expected[i]) {
			t.Errorf("Read(%d): got %x, want %x", i, got[:4], expected[i][:4])
		}
	}
}

func TestWriteBatch_OverwriteExisting(t *testing.T) {
	n := 50
	blockSize := 16
	cfg := Config{NumBlocks: n, BlockSize: blockSize, BucketSize: 5, StashLimit: n + 100}
	oram, err := NewInMemory(cfg)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	// Write some blocks sequentially first
	for i := range n {
		data := bytes.Repeat([]byte{0xAA}, blockSize)
		if _, err := oram.Write(i, data); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}

	// Overwrite all with batch
	items := make([]BatchItem, n)
	expected := make(map[int][]byte)
	for i := range n {
		data := bytes.Repeat([]byte{byte(i + 0x10)}, blockSize)
		items[i] = BatchItem{BlockID: i, Data: data}
		expected[i] = data
	}

	if err := oram.WriteBatch(items); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	for i := range n {
		got, err := oram.Read(i)
		if err != nil {
			t.Fatalf("Read(%d): %v", i, err)
		}
		if !bytes.Equal(got, expected[i]) {
			t.Errorf("Read(%d): got %x, want %x", i, got[:4], expected[i][:4])
		}
	}
}

func TestWriteBatch_Empty(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 5}
	oram, _ := NewInMemory(cfg)
	if err := oram.WriteBatch(nil); err != nil {
		t.Fatalf("WriteBatch(nil): %v", err)
	}
}

func TestWriteBatch_InvalidBlockID(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 5}
	oram, _ := NewInMemory(cfg)

	err := oram.WriteBatch([]BatchItem{{BlockID: -1, Data: make([]byte, 16)}})
	if err != ErrInvalidBlockID {
		t.Errorf("expected ErrInvalidBlockID, got %v", err)
	}

	err = oram.WriteBatch([]BatchItem{{BlockID: 10, Data: make([]byte, 16)}})
	if err != ErrInvalidBlockID {
		t.Errorf("expected ErrInvalidBlockID, got %v", err)
	}
}

func TestWriteBatch_InvalidDataSize(t *testing.T) {
	cfg := Config{NumBlocks: 10, BlockSize: 16, BucketSize: 5}
	oram, _ := NewInMemory(cfg)

	err := oram.WriteBatch([]BatchItem{{BlockID: 0, Data: make([]byte, 8)}})
	if err != ErrInvalidDataSize {
		t.Errorf("expected ErrInvalidDataSize, got %v", err)
	}
}

func TestWriteBatch_ConstantTime(t *testing.T) {
	n := 100
	blockSize := 32
	cfg := Config{
		NumBlocks:    n,
		BlockSize:    blockSize,
		BucketSize:   5,
		StashLimit:   n + 100,
		ConstantTime: true,
	}
	oram, err := NewInMemory(cfg)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	items := make([]BatchItem, n)
	expected := make(map[int][]byte)
	for i := range n {
		data := bytes.Repeat([]byte{byte(i)}, blockSize)
		items[i] = BatchItem{BlockID: i, Data: data}
		expected[i] = data
	}

	if err := oram.WriteBatch(items); err != nil {
		t.Fatalf("WriteBatch (CT): %v", err)
	}

	// Read back and verify (reads also use CT path)
	for i := range n {
		got, err := oram.Read(i)
		if err != nil {
			t.Fatalf("Read(%d): %v", i, err)
		}
		if !bytes.Equal(got, expected[i]) {
			t.Errorf("Read(%d): got %x, want %x", i, got[:4], expected[i][:4])
		}
	}
}

func TestWriteBatch_DuplicateBlockIDs(t *testing.T) {
	blockSize := 16
	cfg := Config{NumBlocks: 10, BlockSize: blockSize, BucketSize: 5, StashLimit: 50}
	oram, err := NewInMemory(cfg)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	// BlockID 3 appears three times — last value should win
	items := []BatchItem{
		{BlockID: 3, Data: bytes.Repeat([]byte{0x01}, blockSize)},
		{BlockID: 5, Data: bytes.Repeat([]byte{0x55}, blockSize)},
		{BlockID: 3, Data: bytes.Repeat([]byte{0x02}, blockSize)},
		{BlockID: 3, Data: bytes.Repeat([]byte{0x03}, blockSize)},
	}

	if err := oram.WriteBatch(items); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	got3, err := oram.Read(3)
	if err != nil {
		t.Fatalf("Read(3): %v", err)
	}
	want3 := bytes.Repeat([]byte{0x03}, blockSize)
	if !bytes.Equal(got3, want3) {
		t.Errorf("Read(3): got %x, want %x", got3[:4], want3[:4])
	}

	got5, err := oram.Read(5)
	if err != nil {
		t.Fatalf("Read(5): %v", err)
	}
	want5 := bytes.Repeat([]byte{0x55}, blockSize)
	if !bytes.Equal(got5, want5) {
		t.Errorf("Read(5): got %x, want %x", got5[:4], want5[:4])
	}
}

func TestWriteBatch_EvictionStrategies(t *testing.T) {
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
			n := 100
			blockSize := 32
			cfg := Config{
				NumBlocks:        n,
				BlockSize:        blockSize,
				BucketSize:       5,
				StashLimit:       n + 100,
				EvictionStrategy: s.strategy,
			}
			oram, err := NewInMemory(cfg)
			if err != nil {
				t.Fatalf("NewInMemory: %v", err)
			}

			items := make([]BatchItem, n)
			expected := make(map[int][]byte)
			for i := range n {
				data := bytes.Repeat([]byte{byte(i)}, blockSize)
				items[i] = BatchItem{BlockID: i, Data: data}
				expected[i] = data
			}

			if err := oram.WriteBatch(items); err != nil {
				t.Fatalf("WriteBatch (%s): %v", s.name, err)
			}

			for i := range n {
				got, err := oram.Read(i)
				if err != nil {
					t.Fatalf("Read(%d): %v", i, err)
				}
				if !bytes.Equal(got, expected[i]) {
					t.Errorf("Read(%d): got %x, want %x", i, got[:4], expected[i][:4])
				}
			}
		})
	}
}

func TestWriteBatch_ConstantTime_OverwriteExisting(t *testing.T) {
	n := 50
	blockSize := 16
	cfg := Config{
		NumBlocks:    n,
		BlockSize:    blockSize,
		BucketSize:   5,
		StashLimit:   n + 100,
		ConstantTime: true,
	}
	oram, err := NewInMemory(cfg)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	// Sequential writes first
	for i := range n {
		if _, err := oram.Write(i, bytes.Repeat([]byte{0xBB}, blockSize)); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}

	// Overwrite with CT batch
	items := make([]BatchItem, n)
	expected := make(map[int][]byte)
	for i := range n {
		data := bytes.Repeat([]byte{byte(i + 0x20)}, blockSize)
		items[i] = BatchItem{BlockID: i, Data: data}
		expected[i] = data
	}

	if err := oram.WriteBatch(items); err != nil {
		t.Fatalf("WriteBatch (CT): %v", err)
	}

	for i := range n {
		got, err := oram.Read(i)
		if err != nil {
			t.Fatalf("Read(%d): %v", i, err)
		}
		if !bytes.Equal(got, expected[i]) {
			t.Errorf("Read(%d): got %x, want %x", i, got[:4], expected[i][:4])
		}
	}
}

// BenchmarkSequentialWrites measures N sequential Write() calls.
func BenchmarkSequentialWrites(b *testing.B) {
	batchSizes := []int{100, 500, 1000, 2000}
	blockSize := 256

	for _, n := range batchSizes {
		// ORAM must have capacity >= n
		cfg := Config{NumBlocks: n, BlockSize: blockSize, BucketSize: 5, StashLimit: n + 100}
		name := fmt.Sprintf("N=%d", n)

		b.Run(name, func(b *testing.B) {
			data := make([]byte, blockSize)
			for i := range data {
				data[i] = byte(i % 256)
			}

			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				b.StopTimer()
				oram, err := NewInMemory(cfg)
				if err != nil {
					b.Fatalf("NewInMemory: %v", err)
				}
				b.StartTimer()

				for i := 0; i < n; i++ {
					if _, err := oram.Write(i, data); err != nil {
						b.Fatalf("Write(%d): %v", i, err)
					}
				}
			}
		})
	}
}

// BenchmarkWriteBatch measures batched WriteBatch() calls.
func BenchmarkWriteBatch(b *testing.B) {
	batchSizes := []int{100, 500, 1000, 2000}
	blockSize := 256

	for _, n := range batchSizes {
		cfg := Config{NumBlocks: n, BlockSize: blockSize, BucketSize: 5, StashLimit: n + 100}
		name := fmt.Sprintf("N=%d", n)

		b.Run(name, func(b *testing.B) {
			data := make([]byte, blockSize)
			for i := range data {
				data[i] = byte(i % 256)
			}

			// Build batch items
			items := make([]BatchItem, n)
			for i := 0; i < n; i++ {
				items[i] = BatchItem{BlockID: i, Data: data}
			}

			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				b.StopTimer()
				oram, err := NewInMemory(cfg)
				if err != nil {
					b.Fatalf("NewInMemory: %v", err)
				}
				b.StartTimer()

				if err := oram.WriteBatch(items); err != nil {
					b.Fatalf("WriteBatch: %v", err)
				}
			}
		})
	}
}
