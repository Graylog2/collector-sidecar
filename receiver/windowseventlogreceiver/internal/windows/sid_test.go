// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSIDCache_Lookup(t *testing.T) {
	var lookupCount atomic.Int32
	cache := newSIDCache(1024, 5*time.Minute, func(sid string) (*SIDInfo, error) {
		lookupCount.Add(1)
		return &SIDInfo{
			UserName: "testuser",
			Domain:   "TESTDOMAIN",
			Type:     "User",
		}, nil
	})

	info, err := cache.resolve("S-1-5-21-123")
	require.NoError(t, err)
	require.Equal(t, "testuser", info.UserName)
	require.Equal(t, int32(1), lookupCount.Load())

	// Second call should hit cache
	info, err = cache.resolve("S-1-5-21-123")
	require.NoError(t, err)
	require.Equal(t, "testuser", info.UserName)
	require.Equal(t, int32(1), lookupCount.Load()) // no new lookup
}

func TestSIDCache_NegativeCache(t *testing.T) {
	var lookupCount atomic.Int32
	cache := newSIDCache(1024, 5*time.Minute, func(sid string) (*SIDInfo, error) {
		lookupCount.Add(1)
		return nil, fmt.Errorf("not found")
	})

	info, err := cache.resolve("S-1-5-21-unknown")
	require.Error(t, err)
	require.Nil(t, info)

	// Second call should hit negative cache
	_, _ = cache.resolve("S-1-5-21-unknown")
	require.Equal(t, int32(1), lookupCount.Load())
}

func TestSIDCache_TTLExpiry(t *testing.T) {
	var lookupCount atomic.Int32
	cache := newSIDCache(1024, 1*time.Millisecond, func(sid string) (*SIDInfo, error) {
		lookupCount.Add(1)
		return &SIDInfo{UserName: "user"}, nil
	})

	cache.resolve("S-1-5-21-123")
	// Wait long enough for TTL to expire
	time.Sleep(50 * time.Millisecond)
	cache.resolve("S-1-5-21-123")
	require.Equal(t, int32(2), lookupCount.Load()) // TTL expired, looked up again
}

func TestSIDCache_Eviction(t *testing.T) {
	var lookupCount atomic.Int32
	cache := newSIDCache(2, 5*time.Minute, func(sid string) (*SIDInfo, error) {
		lookupCount.Add(1)
		return &SIDInfo{UserName: sid}, nil
	})

	cache.resolve("sid1")
	cache.resolve("sid2")
	cache.resolve("sid3") // evicts sid1 (LRU)
	require.Equal(t, int32(3), lookupCount.Load())

	cache.resolve("sid1") // cache miss, re-lookup
	require.Equal(t, int32(4), lookupCount.Load())
}

func TestSIDCache_LRUOrder(t *testing.T) {
	var lookupCount atomic.Int32
	cache := newSIDCache(2, 5*time.Minute, func(sid string) (*SIDInfo, error) {
		lookupCount.Add(1)
		return &SIDInfo{UserName: sid}, nil
	})

	cache.resolve("sid1")
	cache.resolve("sid2")
	cache.resolve("sid1")                          // touches sid1, making sid2 the LRU
	cache.resolve("sid3")                          // should evict sid2, not sid1
	require.Equal(t, int32(3), lookupCount.Load()) // sid1, sid2, sid3

	// sid1 should still be cached
	cache.resolve("sid1")
	require.Equal(t, int32(3), lookupCount.Load())

	// sid2 should have been evicted
	cache.resolve("sid2")
	require.Equal(t, int32(4), lookupCount.Load())
}
