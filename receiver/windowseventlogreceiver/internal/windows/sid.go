// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"container/list"
	"sync"
	"time"
)

// SIDInfo holds resolved SID information.
type SIDInfo struct {
	UserName string
	Domain   string
	Type     string // "User", "Group", "WellKnownGroup", "Computer", etc.
}

// sidLookupFunc is the function signature for SID resolution.
// On Windows this wraps LookupAccountSid; for testing it's injectable.
type sidLookupFunc func(sid string) (*SIDInfo, error)

type sidCacheEntry struct {
	sid       string
	info      *SIDInfo
	err       error
	expiresAt time.Time
}

type sidCache struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	order    *list.List // LRU order (front = most recent)
	maxSize  int
	ttl      time.Duration
	lookupFn sidLookupFunc
}

func newSIDCache(maxSize int, ttl time.Duration, lookupFn sidLookupFunc) *sidCache {
	return &sidCache{
		entries:  make(map[string]*list.Element, maxSize),
		order:    list.New(),
		maxSize:  maxSize,
		ttl:      ttl,
		lookupFn: lookupFn,
	}
}

func (c *sidCache) resolve(sid string) (*SIDInfo, error) {
	c.mu.Lock()

	if elem, ok := c.entries[sid]; ok {
		entry := elem.Value.(*sidCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			c.order.MoveToFront(elem)
			info, err := entry.info, entry.err
			c.mu.Unlock()
			return info, err
		}
		// Expired — remove and re-lookup
		c.removeLocked(elem)
	}

	// Release lock during (potentially slow) lookup
	c.mu.Unlock()
	info, err := c.lookupFn(sid)

	// Re-acquire lock to store result
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if another goroutine resolved while we were looking up
	if elem, ok := c.entries[sid]; ok {
		entry := elem.Value.(*sidCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			c.order.MoveToFront(elem)
			return entry.info, entry.err
		}
		c.removeLocked(elem)
	}

	c.putLocked(sid, info, err)
	return info, err
}

func (c *sidCache) putLocked(sid string, info *SIDInfo, err error) {
	if c.maxSize <= 0 {
		return // caching disabled, resolve-only mode
	}
	entry := &sidCacheEntry{
		sid:       sid,
		info:      info,
		err:       err,
		expiresAt: time.Now().Add(c.ttl),
	}

	if elem, ok := c.entries[sid]; ok {
		elem.Value = entry
		c.order.MoveToFront(elem)
		return
	}

	if c.order.Len() >= c.maxSize {
		// Evict least recently used (back of list)
		oldest := c.order.Back()
		if oldest != nil {
			c.removeLocked(oldest)
		}
	}

	elem := c.order.PushFront(entry)
	c.entries[sid] = elem
}

func (c *sidCache) removeLocked(elem *list.Element) {
	entry := elem.Value.(*sidCacheEntry)
	delete(c.entries, entry.sid)
	c.order.Remove(elem)
}
