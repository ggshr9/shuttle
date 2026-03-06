package crypto

import (
	"encoding/binary"
	"math/rand/v2"
	"sync"
	"time"
)

const (
	bucketSize = 4       // slots per bucket
	fingerSize = 8       // bits per fingerprint
	maxKicks   = 500     // max evictions before declaring full
	defaultCap = 1 << 20 // ~1M entries
)

type cuckooFilter struct {
	buckets    []bucket
	count      int
	numBuckets uint32
}

type bucket [bucketSize]uint8

func newCuckooFilter(capacity int) *cuckooFilter {
	numBuckets := uint32(capacity / bucketSize)
	if numBuckets == 0 {
		numBuckets = 1024
	}
	numBuckets = nextPow2(numBuckets)
	return &cuckooFilter{
		buckets:    make([]bucket, numBuckets),
		numBuckets: numBuckets,
	}
}

func nextPow2(v uint32) uint32 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}

func (cf *cuckooFilter) fingerprint(x uint64) uint8 {
	fp := uint8(x>>32) ^ uint8(x>>24) ^ uint8(x>>16) ^ uint8(x>>8) ^ uint8(x)
	if fp == 0 {
		fp = 1
	}
	return fp
}

func (cf *cuckooFilter) index1(x uint64) uint32 {
	return uint32(x) & (cf.numBuckets - 1)
}

func (cf *cuckooFilter) index2(i1 uint32, fp uint8) uint32 {
	hash := uint32(fp) * 0x5bd1e995
	return (i1 ^ hash) & (cf.numBuckets - 1)
}

func (cf *cuckooFilter) Lookup(x uint64) bool {
	fp := cf.fingerprint(x)
	i1 := cf.index1(x)
	i2 := cf.index2(i1, fp)
	return cf.bucketContains(i1, fp) || cf.bucketContains(i2, fp)
}

func (cf *cuckooFilter) bucketContains(idx uint32, fp uint8) bool {
	b := &cf.buckets[idx]
	for i := 0; i < bucketSize; i++ {
		if b[i] == fp {
			return true
		}
	}
	return false
}

func (cf *cuckooFilter) Insert(x uint64) bool {
	fp := cf.fingerprint(x)
	i1 := cf.index1(x)
	i2 := cf.index2(i1, fp)

	if cf.bucketInsert(i1, fp) {
		cf.count++
		return true
	}
	if cf.bucketInsert(i2, fp) {
		cf.count++
		return true
	}

	idx := i1
	if rand.IntN(2) == 0 {
		idx = i2
	}
	for k := 0; k < maxKicks; k++ {
		slot := rand.IntN(bucketSize)
		old := cf.buckets[idx][slot]
		cf.buckets[idx][slot] = fp
		fp = old
		idx = cf.index2(idx, fp)
		if cf.bucketInsert(idx, fp) {
			cf.count++
			return true
		}
	}
	return false
}

func (cf *cuckooFilter) bucketInsert(idx uint32, fp uint8) bool {
	b := &cf.buckets[idx]
	for i := 0; i < bucketSize; i++ {
		if b[i] == 0 {
			b[i] = fp
			return true
		}
	}
	return false
}

func (cf *cuckooFilter) Count() int { return cf.count }

// ReplayFilter detects replayed nonces using dual-buffer cuckoo filters.
// Uses bounded memory (~2MB), handles millions of nonces.
type ReplayFilter struct {
	mu       sync.Mutex
	current  *cuckooFilter
	previous *cuckooFilter
	window   time.Duration
	lastSwap time.Time
	capacity int
}

func NewReplayFilter(window time.Duration) *ReplayFilter {
	if window == 0 {
		window = 120 * time.Second
	}
	cap := defaultCap
	return &ReplayFilter{
		current:  newCuckooFilter(cap),
		previous: newCuckooFilter(cap),
		window:   window,
		lastSwap: time.Now(),
		capacity: cap,
	}
}

func (rf *ReplayFilter) Check(nonce uint64) bool {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.maybeSwap()
	if rf.current.Lookup(nonce) || rf.previous.Lookup(nonce) {
		return true
	}
	rf.current.Insert(nonce)
	return false
}

func (rf *ReplayFilter) CheckBytes(nonce []byte) bool {
	if len(nonce) < 8 {
		return false
	}
	// Hash all available bytes into a uint64 by XOR-folding 8-byte chunks.
	// This ensures all 32 bytes of a nonce contribute to the fingerprint,
	// not just the first 8.
	var n uint64
	for i := 0; i+8 <= len(nonce); i += 8 {
		n ^= binary.LittleEndian.Uint64(nonce[i : i+8])
	}
	return rf.Check(n)
}

func (rf *ReplayFilter) maybeSwap() {
	if time.Since(rf.lastSwap) < rf.window/2 {
		return
	}
	rf.previous = rf.current
	rf.current = newCuckooFilter(rf.capacity)
	rf.lastSwap = time.Now()
}

func (rf *ReplayFilter) Size() int {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.current.Count() + rf.previous.Count()
}
