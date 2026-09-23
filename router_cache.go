// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"sync"
	"sync/atomic"
)

type routeCacheKey struct {
	method string
	path   string
}

// routeCacheEntry represents either a positive route match or a negative match
// carrying the methods allowed for that concrete path.
type routeCacheEntry struct {
	route   *radixRoute
	values  paramRanges
	allowed allowedMethodSet
}

// RouteCache accelerates repeated concrete dynamic paths. It begins as a
// mutex-protected bounded ring, promotes stable contents to an immutable atomic
// snapshot, and records later traffic in an overlay that may replace the old
// snapshot when the working set changes.
//
// The one-entry hot pointer handles immediate repetition without taking a read
// lock. All cache contents are derived: misses and eviction affect performance,
// never the result returned by the router.
type RouteCache struct {
	methodCache  [routeMethodCount]map[string]routeCacheEntry
	extraCache   map[routeCacheKey]routeCacheEntry
	overlay      [routeMethodCount]map[string]routeCacheEntry
	extraOverlay map[routeCacheKey]routeCacheEntry
	mu           sync.RWMutex
	size         int
	keys         []routeCacheKey
	next         int
	count        uint32
	hits         uint32
	frozenCount  uint32
	admit        [routeCacheAdmissionShards]uint32
	dirty        uint32
	hot          atomic.Pointer[routeCacheHotEntry]
	snapshot     atomic.Pointer[routeCacheReadSnapshot]
	overlayHits  uint32
}

type routeCacheHotEntry struct {
	key   routeCacheKey
	entry routeCacheEntry
}

type routeCacheReadSnapshot struct {
	methodCache [routeMethodCount]map[string]routeCacheEntry
	extraCache  map[routeCacheKey]routeCacheEntry
}

const routeCacheMinRoutes = 64

// A full cache admits one miss per interval in each shard. This protects hot
// entries from one-hit paths while still letting repeatedly missed paths enter.
const routeCacheAdmissionInterval = 4

// Keep this a power of two so routeCacheAdmissionShard can use a mask.
const routeCacheAdmissionShards = 16

// Promote a stable, partially filled cache after two complete hit cycles.
// Keeping one quarter of the cache free prevents read-mostly promotion from
// disabling adaptation when the concrete-path working set exceeds capacity.
const routeCacheFreezeHitCycles = 2
const routeCacheFreezeCapacityNumerator = 3
const routeCacheFreezeCapacityDenominator = 4

// Replace a frozen snapshot when a new working set is at least half its size
// and remains unchanged for the same two complete hit cycles. New overlay
// insertions reset the stability counter, preventing partial phase shifts from
// being promoted prematurely.
const routeCacheRebaseSizeNumerator = 1
const routeCacheRebaseSizeDenominator = 2

// NewRouteCache creates a cache bounded to size concrete method/path entries.
func NewRouteCache(size int) *RouteCache {
	return &RouteCache{size: size}
}

func (rc *RouteCache) get(key routeCacheKey) (routeCacheEntry, bool) {
	return rc.getWithMask(key, methodMaskFor(key.method))
}

// getWithMask reads snapshots without locking. Once a snapshot exists, only
// paths admitted after the freeze require an overlay lock.
func (rc *RouteCache) getWithMask(key routeCacheKey, mask methodMask) (routeCacheEntry, bool) {
	if rc == nil {
		return routeCacheEntry{}, false
	}
	if snapshot := rc.snapshot.Load(); snapshot != nil {
		if entry, ok := snapshot.get(key, mask); ok {
			return entry, true
		}
		if atomic.LoadUint32(&rc.count) == atomic.LoadUint32(&rc.frozenCount) {
			return routeCacheEntry{}, false
		}
		rc.mu.RLock()
		entry, ok := rc.getOverlayLocked(key, mask)
		rc.mu.RUnlock()
		if ok {
			rc.recordOverlayHit(snapshot)
		}
		return entry, ok
	}
	rc.ensureFresh()
	if hot := rc.hot.Load(); hot != nil && hot.key == key {
		return hot.entry, true
	}
	if atomic.LoadUint32(&rc.count) == 0 {
		return routeCacheEntry{}, false
	}
	rc.mu.RLock()
	entry, ok := rc.getLocked(key, mask)
	rc.mu.RUnlock()
	if ok {
		rc.recordHit()
	}
	return entry, ok
}

func (rc *RouteCache) getHot(key routeCacheKey) (routeCacheEntry, bool) {
	if rc == nil || atomic.LoadUint32(&rc.dirty) != 0 {
		return routeCacheEntry{}, false
	}
	if hot := rc.hot.Load(); hot != nil && hot.key == key {
		return hot.entry, true
	}
	return routeCacheEntry{}, false
}

func (rc *RouteCache) set(key routeCacheKey, entry routeCacheEntry) {
	rc.setWithMask(key, methodMaskFor(key.method), entry)
}

// setWithMask inserts into the mutable ring or the post-snapshot overlay. At
// capacity, next identifies the oldest replaceable slot; this is intentionally
// bounded bookkeeping rather than a general-purpose LRU.
func (rc *RouteCache) setWithMask(key routeCacheKey, mask methodMask, entry routeCacheEntry) {
	if rc == nil || rc.size <= 0 {
		return
	}
	rc.ensureFresh()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if snapshot := rc.snapshot.Load(); snapshot != nil {
		if _, exists := snapshot.get(key, mask); exists {
			return
		}
		rc.setOverlayLocked(key, mask, entry)
		return
	}
	if rc.keys == nil {
		rc.keys = make([]routeCacheKey, 0, rc.size)
		rc.next = 0
	}
	if _, exists := rc.getLocked(key, mask); exists {
		rc.setLocked(key, mask, entry)
		if hot := rc.hot.Load(); hot != nil && hot.key == key {
			rc.hot.Store(&routeCacheHotEntry{key: key, entry: entry})
		}
		return
	}
	count := int(atomic.LoadUint32(&rc.count))
	if count < rc.size {
		rc.setLocked(key, mask, entry)
		rc.keys = append(rc.keys, key)
		atomic.StoreUint32(&rc.count, uint32(count+1))
		rc.hot.Store(&routeCacheHotEntry{key: key, entry: entry})
		return
	}

	victim := rc.keys[rc.next]
	rc.deleteLocked(victim, methodMaskFor(victim.method))
	rc.setLocked(key, mask, entry)
	rc.keys[rc.next] = key
	rc.next++
	if rc.next == len(rc.keys) {
		rc.next = 0
	}
	if hot := rc.hot.Load(); hot != nil && hot.key == victim {
		rc.hot.Store(nil)
	}
}

func (rc *RouteCache) setMiss(key routeCacheKey, entry routeCacheEntry) {
	rc.setMissWithMask(key, methodMaskFor(key.method), entry)
}

func (rc *RouteCache) setMissWithMask(key routeCacheKey, mask methodMask, entry routeCacheEntry) {
	if rc == nil || rc.size <= 0 {
		return
	}
	admissionShard := routeCacheAdmissionShard(key)
	if atomic.LoadUint32(&rc.count) >= uint32(rc.size) &&
		atomic.AddUint32(&rc.admit[admissionShard], 1)%routeCacheAdmissionInterval != 0 {
		return
	}
	rc.setWithMask(key, mask, entry)
}

// recordHit freezes a stable, partially filled cache after enough reuse. Full
// caches continue adapting instead of freezing a possibly transient workload.
func (rc *RouteCache) recordHit() {
	count := atomic.LoadUint32(&rc.count)
	if count < routeCacheMinRoutes ||
		uint64(count)*routeCacheFreezeCapacityDenominator >
			uint64(rc.size)*routeCacheFreezeCapacityNumerator {
		return
	}
	hits := atomic.AddUint32(&rc.hits, 1)
	if uint64(hits) < uint64(count)*routeCacheFreezeHitCycles {
		return
	}
	rc.freezeReadSnapshot(count)
}

func (rc *RouteCache) recordOverlayHit(expectedSnapshot *routeCacheReadSnapshot) {
	if rc.snapshot.Load() != expectedSnapshot {
		return
	}
	count := atomic.LoadUint32(&rc.count)
	frozenCount := atomic.LoadUint32(&rc.frozenCount)
	if count <= frozenCount {
		return
	}
	overlayCount := count - frozenCount
	if overlayCount < routeCacheMinRoutes ||
		uint64(overlayCount)*routeCacheFreezeCapacityDenominator >
			uint64(rc.size)*routeCacheFreezeCapacityNumerator ||
		uint64(overlayCount)*routeCacheRebaseSizeDenominator <
			uint64(frozenCount)*routeCacheRebaseSizeNumerator {
		return
	}
	hits := atomic.AddUint32(&rc.overlayHits, 1)
	if uint64(hits) < uint64(overlayCount)*routeCacheFreezeHitCycles {
		return
	}
	rc.rebaseReadSnapshot(expectedSnapshot, overlayCount, frozenCount)
}

func (rc *RouteCache) freezeReadSnapshot(expectedCount uint32) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.snapshot.Load() != nil ||
		atomic.LoadUint32(&rc.dirty) != 0 ||
		atomic.LoadUint32(&rc.count) != expectedCount ||
		uint64(atomic.LoadUint32(&rc.hits)) < uint64(expectedCount)*routeCacheFreezeHitCycles {
		return
	}
	snapshot := &routeCacheReadSnapshot{
		methodCache: rc.methodCache,
		extraCache:  rc.extraCache,
	}
	rc.keys = nil
	rc.next = 0
	atomic.StoreUint32(&rc.frozenCount, expectedCount)
	rc.snapshot.Store(snapshot)
	rc.hot.Store(nil)
}

// rebaseReadSnapshot promotes a sufficiently large and stable overlay, dropping
// the previous snapshot as one operation under the write lock.
func (rc *RouteCache) rebaseReadSnapshot(
	expectedSnapshot *routeCacheReadSnapshot,
	expectedOverlayCount,
	expectedFrozenCount uint32,
) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.snapshot.Load() != expectedSnapshot ||
		atomic.LoadUint32(&rc.dirty) != 0 ||
		atomic.LoadUint32(&rc.frozenCount) != expectedFrozenCount ||
		atomic.LoadUint32(&rc.count) != expectedFrozenCount+expectedOverlayCount ||
		uint64(atomic.LoadUint32(&rc.overlayHits)) <
			uint64(expectedOverlayCount)*routeCacheFreezeHitCycles {
		return
	}

	rc.methodCache = rc.overlay
	rc.extraCache = rc.extraOverlay
	rc.overlay = [routeMethodCount]map[string]routeCacheEntry{}
	rc.extraOverlay = nil
	rc.keys = nil
	rc.next = 0
	atomic.StoreUint32(&rc.count, expectedOverlayCount)
	atomic.StoreUint32(&rc.hits, 0)
	atomic.StoreUint32(&rc.frozenCount, expectedOverlayCount)
	atomic.StoreUint32(&rc.overlayHits, 0)
	rc.snapshot.Store(&routeCacheReadSnapshot{
		methodCache: rc.methodCache,
		extraCache:  rc.extraCache,
	})
	rc.hot.Store(nil)
}

func (rc *RouteCache) getLocked(key routeCacheKey, mask methodMask) (routeCacheEntry, bool) {
	if slot := singleBitIndex(mask); slot >= 0 {
		entry, ok := rc.methodCache[slot][key.path]
		return entry, ok
	}
	entry, ok := rc.extraCache[key]
	return entry, ok
}

func (snapshot *routeCacheReadSnapshot) get(key routeCacheKey, mask methodMask) (routeCacheEntry, bool) {
	if slot := singleBitIndex(mask); slot >= 0 {
		entry, ok := snapshot.methodCache[slot][key.path]
		return entry, ok
	}
	entry, ok := snapshot.extraCache[key]
	return entry, ok
}

func (rc *RouteCache) getOverlayLocked(key routeCacheKey, mask methodMask) (routeCacheEntry, bool) {
	if slot := singleBitIndex(mask); slot >= 0 {
		entry, ok := rc.overlay[slot][key.path]
		return entry, ok
	}
	entry, ok := rc.extraOverlay[key]
	return entry, ok
}

func (rc *RouteCache) setLocked(key routeCacheKey, mask methodMask, entry routeCacheEntry) {
	if slot := singleBitIndex(mask); slot >= 0 {
		cache := rc.methodCache[slot]
		if cache == nil {
			cache = make(map[string]routeCacheEntry)
			rc.methodCache[slot] = cache
		}
		cache[key.path] = entry
		return
	}
	if rc.extraCache == nil {
		rc.extraCache = make(map[routeCacheKey]routeCacheEntry)
	}
	rc.extraCache[key] = entry
}

func (rc *RouteCache) setOverlayLocked(key routeCacheKey, mask methodMask, entry routeCacheEntry) {
	if _, exists := rc.getOverlayLocked(key, mask); exists {
		rc.storeOverlayLocked(key, mask, entry)
		return
	}
	count := int(atomic.LoadUint32(&rc.count))
	if count < rc.size {
		rc.storeOverlayLocked(key, mask, entry)
		rc.keys = append(rc.keys, key)
		atomic.StoreUint32(&rc.count, uint32(count+1))
		atomic.StoreUint32(&rc.overlayHits, 0)
		rc.hot.Store(&routeCacheHotEntry{key: key, entry: entry})
		return
	}
	if len(rc.keys) == 0 {
		return
	}
	victim := rc.keys[rc.next]
	rc.deleteOverlayLocked(victim, methodMaskFor(victim.method))
	rc.storeOverlayLocked(key, mask, entry)
	rc.keys[rc.next] = key
	atomic.StoreUint32(&rc.overlayHits, 0)
	rc.next++
	if rc.next == len(rc.keys) {
		rc.next = 0
	}
	if hot := rc.hot.Load(); hot != nil && hot.key == victim {
		rc.hot.Store(nil)
	}
}

func (rc *RouteCache) storeOverlayLocked(key routeCacheKey, mask methodMask, entry routeCacheEntry) {
	if slot := singleBitIndex(mask); slot >= 0 {
		cache := rc.overlay[slot]
		if cache == nil {
			cache = make(map[string]routeCacheEntry)
			rc.overlay[slot] = cache
		}
		cache[key.path] = entry
		return
	}
	if rc.extraOverlay == nil {
		rc.extraOverlay = make(map[routeCacheKey]routeCacheEntry)
	}
	rc.extraOverlay[key] = entry
}

func (rc *RouteCache) deleteLocked(key routeCacheKey, mask methodMask) {
	if slot := singleBitIndex(mask); slot >= 0 {
		delete(rc.methodCache[slot], key.path)
		return
	}
	delete(rc.extraCache, key)
}

func (rc *RouteCache) deleteOverlayLocked(key routeCacheKey, mask methodMask) {
	if slot := singleBitIndex(mask); slot >= 0 {
		delete(rc.overlay[slot], key.path)
		return
	}
	delete(rc.extraOverlay, key)
}

// routeCacheAdmissionShard is a cheap dispersion function, not a hash table
// identity. Collisions merely share an admission counter and remain correct.
func routeCacheAdmissionShard(key routeCacheKey) int {
	path := key.path
	hash := uint32(len(path))*16777619 ^ uint32(len(key.method))
	if len(path) != 0 {
		hash = (hash ^ uint32(path[0])) * 16777619
		hash = (hash ^ uint32(path[len(path)/2])) * 16777619
		hash = (hash ^ uint32(path[len(path)-1])) * 16777619
	}
	if len(key.method) != 0 {
		hash = (hash ^ uint32(key.method[0])) * 16777619
	}
	return int(hash & (routeCacheAdmissionShards - 1))
}

// invalidate makes lock-free state unreachable immediately. The next cache
// operation clears mutable maps under the lock, keeping registration cheap.
func (rc *RouteCache) invalidate() {
	if rc == nil {
		return
	}
	atomic.StoreUint32(&rc.dirty, 1)
	rc.snapshot.Store(nil)
	rc.hot.Store(nil)
}

func (rc *RouteCache) ensureFresh() {
	if rc == nil || atomic.LoadUint32(&rc.dirty) == 0 {
		return
	}
	rc.mu.Lock()
	if atomic.LoadUint32(&rc.dirty) != 0 {
		rc.methodCache = [routeMethodCount]map[string]routeCacheEntry{}
		rc.extraCache = nil
		rc.overlay = [routeMethodCount]map[string]routeCacheEntry{}
		rc.extraOverlay = nil
		rc.keys = nil
		rc.next = 0
		atomic.StoreUint32(&rc.count, 0)
		atomic.StoreUint32(&rc.hits, 0)
		atomic.StoreUint32(&rc.frozenCount, 0)
		atomic.StoreUint32(&rc.overlayHits, 0)
		for i := range rc.admit {
			atomic.StoreUint32(&rc.admit[i], 0)
		}
		atomic.StoreUint32(&rc.dirty, 0)
		rc.snapshot.Store(nil)
		rc.hot.Store(nil)
	}
	rc.mu.Unlock()
}
