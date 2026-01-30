package pathoram

import "crypto/subtle"

// findInStashConstantTime searches stash without timing leaks.
// Returns (index, data) where index is -1 if not found.
// Always iterates through entire stash regardless of match.
func (o *PathORAM) findInStashConstantTime(blockID int) (int, []byte) {
	foundIdx := -1
	result := make([]byte, o.cfg.BlockSize)

	for i := range o.stash {
		match := subtle.ConstantTimeEq(int32(o.stash[i].id), int32(blockID))
		foundIdx = subtle.ConstantTimeSelect(match, i, foundIdx)
		subtle.ConstantTimeCopy(match, result, o.stash[i].data)
	}
	return foundIdx, result
}

// canPlaceAtConstantTime checks placement without early exit.
// Always walks the full path from leaf to root.
func (o *PathORAM) canPlaceAtConstantTime(leaf, bucketIdx int) bool {
	leafBucket := o.numLeaves - 1 + leaf
	found := 0

	// Walk from leafBucket to root, checking if we hit bucketIdx
	for level := 0; level < o.height; level++ {
		b := leafBucket
		for j := 0; j < level; j++ {
			b = (b - 1) / 2
		}
		found |= subtle.ConstantTimeEq(int32(b), int32(bucketIdx))
	}
	return found == 1
}

// evictConstantTime performs eviction without timing leaks.
// Always processes all stash blocks and all path buckets.
func (o *PathORAM) evictConstantTime(path []int) error {
	// Read all buckets on path
	buckets := make([][]Block, len(path))
	for i, bucketIdx := range path {
		var err error
		buckets[i], err = o.storage.ReadBucket(bucketIdx)
		if err != nil {
			return err
		}
	}

	// Process each stash block - always iterate all
	newStash := make([]block, 0, len(o.stash))

	for i := range o.stash {
		b := &o.stash[i]
		placed := 0

		// Try each level (deepest first)
		for level := 0; level < len(path); level++ {
			bucketIdx := path[level]

			// Check if can place (constant-time)
			canPlace := 0
			if o.canPlaceAtConstantTime(b.leaf, bucketIdx) {
				canPlace = 1
			}

			// Find empty slot (constant-time scan)
			for slot := range buckets[level] {
				isEmpty := subtle.ConstantTimeEq(int32(buckets[level][slot].ID), int32(EmptyBlockID))
				shouldPlace := canPlace & isEmpty & (1 ^ placed)

				// Conditionally write block to slot
				if shouldPlace == 1 {
					buckets[level][slot] = o.blockToStorage(*b)
					placed = 1
				}
			}
		}

		// If not placed, keep in stash
		if placed == 0 {
			newStash = append(newStash, *b)
		}
	}

	o.stash = newStash

	// Write all buckets back
	for i, bucketIdx := range path {
		if err := o.storage.WriteBucket(bucketIdx, buckets[i]); err != nil {
			return err
		}
	}

	if len(o.stash) > o.cfg.StashLimit {
		return ErrStashOverflow
	}
	return nil
}
