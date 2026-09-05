package tui

import "github.com/verdantran/wakeart/internal/render"

type cacheEntry struct {
	frame   *render.Frame
	cropped bool
}

// frameCache bounds the composed-frame cache so a large deck cannot grow
// memory without limit. Eviction is by least recent use: a scene with more
// frames than the cache holds walks them in order, and evicting by insertion
// would throw away exactly the entry wanted next.
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
	if ok {
		c.touch(k)
	}
	return f, ok
}

// touch moves a key to the most-recent end. The cache holds tens of entries,
// so the linear scan is cheaper than a second index.
func (c *frameCache) touch(k string) {
	for i, o := range c.order {
		if o == k {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, k)
			return
		}
	}
}

func (c *frameCache) put(k string, f cacheEntry) {
	if _, ok := c.items[k]; ok {
		c.touch(k)
	} else {
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
