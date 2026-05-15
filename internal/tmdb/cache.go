package tmdb

import (
	"context"
	"sync"
	"time"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
)

// Cache wraps a Client with three independent in-memory caches (one per
// enrichment group). Each cache uses a 24h-style TTL. Errors from the
// upstream client are NEVER cached. The Cache implements routing.Enricher.
type Cache struct {
	upstream *Client
	ttl      time.Duration
	now      func() time.Time // injectable for tests
	primary  sync.Map         // key: cacheKey -> primaryEntry
	keywords sync.Map         // key: cacheKey -> keywordsEntry
	rating   sync.Map         // key: cacheKey -> ratingEntry
}

// Compile-time assertion: Cache implements routing.Enricher.
var _ routing.Enricher = (*Cache)(nil)

type cacheKey struct {
	MediaType string
	ID        int
}

type primaryEntry struct {
	V  *routing.TMDBPrimary
	At time.Time
}

type keywordsEntry struct {
	V  []string
	At time.Time
}

type ratingEntry struct {
	V  string
	At time.Time
}

// NewCache returns a Cache wrapping upstream with the given TTL. ttl<=0
// disables caching (every call hits upstream).
func NewCache(upstream *Client, ttl time.Duration) *Cache {
	return &Cache{upstream: upstream, ttl: ttl, now: time.Now}
}

// SetNow is a test seam — do not use in production code.
func (c *Cache) SetNow(f func() time.Time) { c.now = f }

// Primary fetches (and caches) the top-level TMDB metadata for a movie or TV
// show. Within TTL, repeated calls with the same (mediaType, tmdbID) return
// the cached value without hitting the upstream client.
func (c *Cache) Primary(ctx context.Context, mediaType string, tmdbID int) (*routing.TMDBPrimary, error) {
	k := cacheKey{mediaType, tmdbID}
	if c.ttl > 0 {
		if v, ok := c.primary.Load(k); ok {
			e := v.(primaryEntry)
			if c.now().Sub(e.At) < c.ttl {
				return e.V, nil
			}
		}
	}
	v, err := c.upstream.Primary(ctx, mediaType, tmdbID)
	if err != nil {
		return nil, err
	}
	if c.ttl > 0 {
		c.primary.Store(k, primaryEntry{V: v, At: c.now()})
	}
	return v, nil
}

// Keywords fetches (and caches) keyword names for a movie or TV show.
// Within TTL, repeated calls with the same (mediaType, tmdbID) return
// the cached value without hitting the upstream client.
func (c *Cache) Keywords(ctx context.Context, mediaType string, tmdbID int) ([]string, error) {
	k := cacheKey{mediaType, tmdbID}
	if c.ttl > 0 {
		if v, ok := c.keywords.Load(k); ok {
			e := v.(keywordsEntry)
			if c.now().Sub(e.At) < c.ttl {
				return e.V, nil
			}
		}
	}
	v, err := c.upstream.Keywords(ctx, mediaType, tmdbID)
	if err != nil {
		return nil, err
	}
	if c.ttl > 0 {
		c.keywords.Store(k, keywordsEntry{V: v, At: c.now()})
	}
	return v, nil
}

// ContentRating fetches (and caches) the US content rating for a movie or TV
// show. Within TTL, repeated calls with the same (mediaType, tmdbID) return
// the cached value without hitting the upstream client.
func (c *Cache) ContentRating(ctx context.Context, mediaType string, tmdbID int) (string, error) {
	k := cacheKey{mediaType, tmdbID}
	if c.ttl > 0 {
		if v, ok := c.rating.Load(k); ok {
			e := v.(ratingEntry)
			if c.now().Sub(e.At) < c.ttl {
				return e.V, nil
			}
		}
	}
	v, err := c.upstream.ContentRating(ctx, mediaType, tmdbID)
	if err != nil {
		return "", err
	}
	if c.ttl > 0 {
		c.rating.Store(k, ratingEntry{V: v, At: c.now()})
	}
	return v, nil
}
