package pathoram

import "crypto/subtle"

// BatchItem represents a single block write in a batch operation.
type BatchItem struct {
	BlockID int
	Data    []byte
}

// deduplicateBatchItems keeps only the last occurrence of each BlockID.
func deduplicateBatchItems(items []BatchItem) []BatchItem {
	last := make(map[int]int, len(items))
	for i, item := range items {
		last[item.BlockID] = i
	}
	if len(last) == len(items) {
		return items
	}
	deduped := make([]BatchItem, 0, len(last))
	for i, item := range items {
		if last[item.BlockID] == i {
			deduped = append(deduped, item)
		}
	}
	return deduped
}

// WriteBatch writes multiple blocks in a single batched operation.
// This is significantly faster than sequential Write() calls because:
//   - Shared ancestor buckets are read/written once instead of up to N times
//   - Stash eviction operates over the union of all paths (deeper placement)
//   - Stash scanning happens once instead of N times
//
// All blocks must have valid IDs (0 to NumBlocks-1) and data of exactly BlockSize bytes.
// Duplicate BlockIDs are deduplicated (last-writer-wins).
// Note: this operation is NOT access-pattern oblivious — an observer can distinguish
// a batch from N independent accesses. Use sequential Write() for obliviousness.
func (o *PathORAM) WriteBatch(items []BatchItem) error {
	if len(items) == 0 {
		return nil
	}

	// Validate all items upfront
	for _, item := range items {
		if item.BlockID < 0 || item.BlockID >= o.cfg.NumBlocks {
			return ErrInvalidBlockID
		}
		if len(item.Data) != o.cfg.BlockSize {
			return ErrInvalidDataSize
		}
	}

	items = deduplicateBatchItems(items)

	// Phase 1: Remap all blocks and collect old paths
	paths := make([][]int, len(items))
	for i, item := range items {
		oldLeaf, exists := o.posMap.Get(item.BlockID)
		if !exists {
			oldLeaf = o.randomLeaf()
		}
		o.posMap.Set(item.BlockID, o.randomLeaf())
		paths[i] = o.Path(oldLeaf)
	}

	// Phase 2: Read all unique buckets into stash.
	// Retain emptied bucket data for direct reuse in eviction (no double-read).
	bucketData := make(map[int][]Block)
	for _, path := range paths {
		for _, bucketIdx := range path {
			if _, seen := bucketData[bucketIdx]; seen {
				continue
			}

			bucket, err := o.storage.ReadBucket(bucketIdx)
			if err != nil {
				return err
			}
			for j := range bucket {
				if bucket[j].ID != EmptyBlockID {
					plaintext, err := o.encrypt.Decrypt(bucket[j].ID, bucket[j].Leaf, bucket[j].Data)
					if err != nil {
						return err
					}
					o.stash = append(o.stash, block{
						id:   bucket[j].ID,
						leaf: bucket[j].Leaf,
						data: plaintext,
					})
					bucket[j] = Block{
						ID:   EmptyBlockID,
						Leaf: -1,
						Data: make([]byte, len(bucket[j].Data)),
					}
				}
			}
			bucketData[bucketIdx] = bucket
		}
	}

	// Phase 3: Update/insert all batch blocks in stash
	if o.cfg.ConstantTime {
		o.updateStashBatchCT(items)
	} else {
		o.updateStashBatch(items)
	}

	// Phase 4: Eviction — respects configured strategy and ConstantTime mode
	if o.cfg.ConstantTime {
		return o.evictMultiPathCT(paths, bucketData)
	}
	return o.evictMultiPathWithStrategy(paths, bucketData)
}

// updateStashBatch updates stash with batch items using O(1) hash lookup.
func (o *PathORAM) updateStashBatch(items []BatchItem) {
	stashIdx := make(map[int]int, len(o.stash))
	for i, b := range o.stash {
		stashIdx[b.id] = i
	}

	for _, item := range items {
		newLeaf, _ := o.posMap.Get(item.BlockID)
		if idx, found := stashIdx[item.BlockID]; found {
			o.stash[idx].leaf = newLeaf
			copy(o.stash[idx].data, item.Data)
		} else {
			newBlock := block{
				id:   item.BlockID,
				leaf: newLeaf,
				data: make([]byte, o.cfg.BlockSize),
			}
			copy(newBlock.data, item.Data)
			stashIdx[item.BlockID] = len(o.stash)
			o.stash = append(o.stash, newBlock)
		}
	}
}

// updateStashBatchCT updates stash with batch items using constant-time scan.
// For each item, scans the entire stash to find the block without early exit.
// Note: the found/not-found branch is consistent with access()'s CT path.
func (o *PathORAM) updateStashBatchCT(items []BatchItem) {
	for _, item := range items {
		newLeaf, _ := o.posMap.Get(item.BlockID)

		foundIdx := -1
		for j := range o.stash {
			match := subtle.ConstantTimeEq(int32(o.stash[j].id), int32(item.BlockID))
			foundIdx = subtle.ConstantTimeSelect(match, j, foundIdx)
		}

		if foundIdx >= 0 {
			o.stash[foundIdx].leaf = newLeaf
			copy(o.stash[foundIdx].data, item.Data)
		} else {
			newBlock := block{
				id:   item.BlockID,
				leaf: newLeaf,
				data: make([]byte, o.cfg.BlockSize),
			}
			copy(newBlock.data, item.Data)
			o.stash = append(o.stash, newBlock)
		}
	}
}

// buildStashPathSets precomputes, for each stash block, the set of bucket indices
// on its assigned leaf's path. Enables O(1) canPlaceAt checks.
func (o *PathORAM) buildStashPathSets() []map[int]bool {
	sets := make([]map[int]bool, len(o.stash))
	for i, b := range o.stash {
		path := o.Path(b.leaf)
		set := make(map[int]bool, len(path))
		for _, idx := range path {
			set[idx] = true
		}
		sets[i] = set
	}
	return sets
}

// evictMultiPathWithStrategy dispatches to the configured eviction strategy
// for multi-path batch eviction, mirroring evictWithStrategy for single-path.
func (o *PathORAM) evictMultiPathWithStrategy(paths [][]int, bucketData map[int][]Block) error {
	switch o.cfg.EvictionStrategy {
	case EvictGreedyByDepth:
		return o.evictMultiPath(paths, bucketData)
	case EvictDeterministicTwoPath:
		if err := o.evictMultiPath(paths, bucketData); err != nil {
			return err
		}
		// Background eviction on a random second path (same as single-access two-path)
		secondPath := o.Path(o.randomLeaf())
		if err := o.readPathIntoStash(secondPath); err != nil {
			return err
		}
		return o.evictGreedyByDepth(secondPath)
	default: // EvictLevelByLevel
		return o.evictMultiPathLevelByLevel(paths, bucketData)
	}
}

// pathSetThreshold is the stash size above which we precompute path sets
// for O(1) canPlaceAt lookups. Below this, direct tree-walk is faster
// because it avoids map allocation overhead.
const pathSetThreshold = 128

// canPlaceBatch checks if a block assigned to the given leaf can be placed
// in bucketIdx. Uses precomputed path sets when available, otherwise falls
// back to direct tree-walk.
func canPlaceBatch(o *PathORAM, pathSets []map[int]bool, idx int, leaf, bucketIdx int) bool {
	if pathSets != nil {
		return pathSets[idx][bucketIdx]
	}
	return o.canPlaceAt(leaf, bucketIdx)
}

// evictMultiPath performs greedy-by-depth eviction across the union of multiple paths.
// For each stash block, tries the deepest valid bucket across any path.
// Precomputes path sets for O(1) placement checks when stash is large enough
// to amortize the allocation cost.
func (o *PathORAM) evictMultiPath(paths [][]int, bucketData map[int][]Block) error {
	if len(paths) == 0 {
		return nil
	}
	height := len(paths[0])

	var pathSets []map[int]bool
	if len(o.stash) > pathSetThreshold {
		pathSets = o.buildStashPathSets()
	}

	i := 0
	for i < len(o.stash) {
		placed := false

		for level := 0; level < height && !placed; level++ {
			for _, path := range paths {
				bucketIdx := path[level]
				if !canPlaceBatch(o, pathSets, i, o.stash[i].leaf, bucketIdx) {
					continue
				}
				bucket := bucketData[bucketIdx]
				for slot := range bucket {
					if bucket[slot].ID == EmptyBlockID {
						bucket[slot] = o.blockToStorage(o.stash[i])
						last := len(o.stash) - 1
						o.stash[i] = o.stash[last]
						o.stash = o.stash[:last]
						if pathSets != nil {
							pathSets[i] = pathSets[last]
							pathSets = pathSets[:last]
						}
						placed = true
						break
					}
				}
				if placed {
					break
				}
			}
		}
		if !placed {
			i++
		}
	}

	return o.writeBackAndCheckStash(bucketData)
}

// evictMultiPathLevelByLevel performs level-by-level eviction across the union of
// multiple paths. For each level (deepest first), fills all available slots across
// all paths before moving to the next level. This prioritizes blocks with the most
// constrained placement, reducing stash pressure compared to per-block greedy.
func (o *PathORAM) evictMultiPathLevelByLevel(paths [][]int, bucketData map[int][]Block) error {
	if len(paths) == 0 {
		return nil
	}
	height := len(paths[0])

	var pathSets []map[int]bool
	if len(o.stash) > pathSetThreshold {
		pathSets = o.buildStashPathSets()
	}

	for level := 0; level < height; level++ {
		seen := make(map[int]bool)
		for _, path := range paths {
			bucketIdx := path[level]
			if seen[bucketIdx] {
				continue
			}
			seen[bucketIdx] = true

			bucket := bucketData[bucketIdx]
			for slot := range bucket {
				if bucket[slot].ID != EmptyBlockID {
					continue
				}
				for i := 0; i < len(o.stash); i++ {
					if canPlaceBatch(o, pathSets, i, o.stash[i].leaf, bucketIdx) {
						bucket[slot] = o.blockToStorage(o.stash[i])
						last := len(o.stash) - 1
						o.stash[i] = o.stash[last]
						o.stash = o.stash[:last]
						if pathSets != nil {
							pathSets[i] = pathSets[last]
							pathSets = pathSets[:last]
						}
						break
					}
				}
			}
		}
	}

	return o.writeBackAndCheckStash(bucketData)
}

// evictMultiPathCT performs constant-time multi-path eviction.
// Always iterates all stash blocks × all levels × all paths × all slots.
// Uses precomputed path slices for O(H) placement checks (vs O(H²) with
// canPlaceAtConstantTime).
//
// Known limitation (consistent with evictConstantTime): the blockToStorage call
// inside the shouldPlace branch involves encryption, which is not constant-time.
func (o *PathORAM) evictMultiPathCT(paths [][]int, bucketData map[int][]Block) error {
	if len(paths) == 0 {
		return nil
	}
	height := len(paths[0])

	// Precompute each stash block's path as a slice (CT-safe: linear scan, no hash map)
	stashPaths := make([][]int, len(o.stash))
	for i, b := range o.stash {
		stashPaths[i] = o.Path(b.leaf)
	}

	newStash := make([]block, 0, len(o.stash))

	for i := range o.stash {
		b := &o.stash[i]
		placed := 0

		for level := 0; level < height; level++ {
			for _, path := range paths {
				bucketIdx := path[level]

				// CT canPlaceAt via precomputed path scan: O(H) per check
				canPlace := 0
				for _, pb := range stashPaths[i] {
					canPlace |= subtle.ConstantTimeEq(int32(pb), int32(bucketIdx))
				}

				bucket := bucketData[bucketIdx]
				for slot := range bucket {
					isEmpty := subtle.ConstantTimeEq(int32(bucket[slot].ID), int32(EmptyBlockID))
					shouldPlace := canPlace & isEmpty & (1 ^ placed)

					if shouldPlace == 1 {
						bucket[slot] = o.blockToStorage(*b)
						placed = 1
					}
				}
			}
		}

		if placed == 0 {
			newStash = append(newStash, *b)
		}
	}

	o.stash = newStash
	return o.writeBackAndCheckStash(bucketData)
}

// writeBackAndCheckStash writes all buckets in bucketData to storage
// and checks for stash overflow.
func (o *PathORAM) writeBackAndCheckStash(bucketData map[int][]Block) error {
	for bucketIdx, bucket := range bucketData {
		if err := o.storage.WriteBucket(bucketIdx, bucket); err != nil {
			return err
		}
	}

	if len(o.stash) > o.cfg.StashLimit {
		return ErrStashOverflow
	}
	return nil
}
