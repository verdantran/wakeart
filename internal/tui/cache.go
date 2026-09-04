package tui

import "github.com/verdantran/wakeart/internal/render"

type cacheEntry struct {
	frame   *render.Frame
	cropped bool
}

// frameCache bounds the composed-frame cache so a large deck cannot grow
// memory without limit.
type frameCache struct {
	max   int
	items map[string]cacheEntry
	order []string
}

func newFrameCache(max int) *frameCache {
	return &frameCache{max: max, items: map[string]cacheEntry{}}
}

func (c *frameCache) get(k string) (cacheEntry, bool) {
	f, ok := c.items[k]
	return f, ok
}

func (c *frameCache) put(k string, f cacheEntry) {
	if _, ok := c.items[k]; !ok {
		c.order = append(c.order, k)
	}
	c.items[k] = f
	for len(c.order) > c.max {
		delete(c.items, c.order[0])
		c.order = c.order[1:]
	}
}

func (c *frameCache) reset() {
	c.items = map[string]cacheEntry{}
	c.order = c.order[:0]
}
