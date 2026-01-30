package pathoram

// evictWithStrategy dispatches to the configured eviction strategy.
func (o *PathORAM) evictWithStrategy(path []int) error {
	switch o.cfg.EvictionStrategy {
	case EvictGreedyByDepth:
		return o.evictGreedyByDepth(path)
	case EvictDeterministicTwoPath:
		if err := o.evictGreedyByDepth(path); err != nil {
			return err
		}
		// Read second path into stash, then evict along it
		secondPath := o.Path(o.randomLeaf())
		if err := o.readPathIntoStash(secondPath); err != nil {
			return err
		}
		return o.evictGreedyByDepth(secondPath)
	default: // EvictLevelByLevel
		return o.evict(path)
	}
}

// evict writes blocks from stash back to the path using level-by-level strategy.
func (o *PathORAM) evict(path []int) error {
	// For each level from leaf to root, try to place blocks
	for level := 0; level < len(path); level++ {
		bucketIdx := path[level]

		bucket, err := o.storage.ReadBucket(bucketIdx)
		if err != nil {
			return err
		}

		modified := false
		// Find blocks in stash that can go to this bucket
		for slot := 0; slot < o.cfg.BucketSize; slot++ {
			if bucket[slot].ID != EmptyBlockID {
				continue // slot occupied
			}
			// Find a block whose path contains this bucket
			for i := 0; i < len(o.stash); i++ {
				b := &o.stash[i]
				if o.canPlaceAt(b.leaf, bucketIdx) {
					bucket[slot] = o.blockToStorage(*b)
					// Remove from stash
					o.stash = append(o.stash[:i], o.stash[i+1:]...)
					modified = true
					break
				}
			}
		}

		if modified {
			if err := o.storage.WriteBucket(bucketIdx, bucket); err != nil {
				return err
			}
		}
	}

	// Check stash overflow
	if len(o.stash) > o.cfg.StashLimit {
		return ErrStashOverflow
	}
	return nil
}

// evictGreedyByDepth places each stash block at its deepest possible level.
// This minimizes stash pressure by keeping blocks as close to leaves as possible.
func (o *PathORAM) evictGreedyByDepth(path []int) error {
	// Read all buckets on path
	buckets := make([][]Block, len(path))
	for i, bucketIdx := range path {
		var err error
		buckets[i], err = o.storage.ReadBucket(bucketIdx)
		if err != nil {
			return err
		}
	}

	i := 0
	for i < len(o.stash) {
		b := &o.stash[i]
		placed := false

		// Try deepest level first (leaf = path[0], root = path[len-1])
		for level := 0; level < len(path); level++ {
			bucketIdx := path[level]
			if !o.canPlaceAt(b.leaf, bucketIdx) {
				continue
			}
			// Find empty slot in this bucket
			for slot := range buckets[level] {
				if buckets[level][slot].ID == EmptyBlockID {
					buckets[level][slot] = o.blockToStorage(*b)
					// Remove from stash (swap with last, shrink)
					o.stash[i] = o.stash[len(o.stash)-1]
					o.stash = o.stash[:len(o.stash)-1]
					placed = true
					break
				}
			}
			if placed {
				break
			}
		}
		if !placed {
			i++
		}
	}

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
