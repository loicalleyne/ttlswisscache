package ttlswisscache

import (
	"time"

	csmap "github.com/mhmtszr/concurrent-swiss-map"
)

const defaultCapacity = 64 // Just to avoid extra allocations in most of the cases.

// Cache represents key-value storage.
type Cache struct {
	done  chan struct{}
	items *csmap.CsMap[uint64, item]
}

type item struct {
	deadline int64 // Unix nano
	value    interface{}
}

// New creates key-value storage.
// resolution – configures cleanup manager.
// Cleanup operation locks storage so think twice before setting it to small value.
func New(resolution time.Duration) *Cache {
	items := csmap.Create[uint64, item](
		csmap.WithShardCount[uint64, item](32),
		csmap.WithSize[uint64, item](defaultCapacity),
	)
	c := &Cache{
		done:  make(chan struct{}),
		items: items,
	}

	go cleaner(c, resolution)

	return c
}

// Get returns stored record.
// The first returned variable is a stored value.
// The second one is an existence flag like in the map.
func (c *Cache) Get(key uint64) (interface{}, bool) {
	cacheItem, ok := c.items.Load(key)
	if !ok {
		return nil, false
	}
	return cacheItem.value, true
}

// Set adds value to the cache with given ttl.
// ttl value should be a multiple of the resolution time value.
func (c *Cache) Set(key uint64, value interface{}, ttl time.Duration) {
	cacheItem := item{
		deadline: time.Now().UnixNano() + int64(ttl),
		value:    value,
	}
	c.items.Store(key, cacheItem)
}

// Delete removes record from storage.
func (c *Cache) Delete(key uint64) {
	c.items.Delete(key)
}

// Clear removes all items from storage and leaves the cleanup manager running.
func (c *Cache) Clear() {
	c.items.Clear()
}

// Close stops cleanup manager and removes records from storage.
func (c *Cache) Close() error {
	close(c.done)
	c.items.Clear()
	return nil
}

// cleanup removes outdated items from the storage.
// It triggers stop the world for the cache.
func (c *Cache) cleanup() {
	now := time.Now().UnixNano()
	k := make([]uint64, c.items.Count())
	i := 0
	c.items.Range(func(key uint64, value item) (stop bool) {
		if value.deadline < now {
			k[i] = key
			i++
		}
		return false
	})
	for _, d := range k {
		c.items.Delete(d)
	}
}

func cleaner(c *Cache, resolution time.Duration) {
	ticker := time.NewTicker(resolution)

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.done:
			ticker.Stop()
			return
		}
	}
}
