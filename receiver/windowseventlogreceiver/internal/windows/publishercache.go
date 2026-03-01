// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import (
	"go.uber.org/multierr"
)

// publisherEntry holds a publisher and its associated caches.
type publisherEntry struct {
	publisher     Publisher
	templates     *templateCache    // message templates for this provider
	paramMessages map[uint32]string // %%ID → resolved string cache
}

type publisherCache struct {
	cache  map[string]*publisherEntry
	locale uint32
}

func newPublisherCache(locale uint32) publisherCache {
	return publisherCache{
		cache:  make(map[string]*publisherEntry),
		locale: locale,
	}
}

func (c *publisherCache) get(provider string) (Publisher, error) {
	entry, ok := c.cache[provider]
	if ok {
		return entry.publisher, nil
	}

	newEntry := &publisherEntry{
		publisher:     NewPublisher(),
		templates:     newTemplateCache(),
		paramMessages: make(map[uint32]string),
	}

	var err error
	if provider != "" {
		// If the provider is empty, there is nothing to be formatted on the event
		// keep the invalid publisher in the cache. See issue #35135
		err = newEntry.publisher.Open(provider, c.locale)
	}

	// Always store the entry even if there was an error opening it.
	c.cache[provider] = newEntry

	return newEntry.publisher, err
}

func (c *publisherCache) getEntry(provider string) (*publisherEntry, error) {
	// Ensure the entry exists via get (which handles first-open)
	_, err := c.get(provider)
	return c.cache[provider], err
}

func (c *publisherCache) evictAll() error {
	var errs error
	for _, entry := range c.cache {
		if entry.publisher.Valid() {
			errs = multierr.Append(errs, entry.publisher.Close())
		}
	}

	c.cache = make(map[string]*publisherEntry)
	return errs
}
