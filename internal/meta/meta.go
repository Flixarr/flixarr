// Package meta fetches movie and series data from the Flixarr API (api.flixarr.app).
// Installs never talk to TMDB directly. Answers are saved in the app's database (the cache)
// so repeat lookups don't leave the server.
package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// Kinds of saved answers, used for durations, counts and per-kind flushing.
const (
	KindTitles   = "titles"   // movie, series, season and collection pages
	KindPeople   = "people"   // person pages
	KindLists    = "lists"    // Discover rows, browse pages, genres
	KindSearches = "searches" // search results
)

// Kinds in display order.
var Kinds = []string{KindTitles, KindPeople, KindLists, KindSearches}

// Config is the cache's settings (Settings → Cache).
type Config struct {
	Enabled     bool `json:"enabled"`
	PagesMin    int  `json:"pagesMinutes"` // titles + people; 0 = automatic (the API's time for each title)
	ListsMin    int  `json:"listsMinutes"`
	SearchesMin int  `json:"searchesMinutes"`
	MaxResults  int  `json:"maxResults"`
}

// DefaultConfig is what a new install starts with.
var DefaultConfig = Config{Enabled: true, PagesMin: Auto, ListsMin: 3 * 60, SearchesMin: 60, MaxResults: 10000}

// Auto keeps each title page as long as the API says it stays fresh: hours for airing series and
// new or upcoming movies, days for ended series, older movies and people.
const Auto = 0

// Settings keys.
const (
	keyEnabled  = "cache_enabled"
	keyPages    = "cache_pages_minutes"
	keyLists    = "cache_lists_minutes"
	keySearches = "cache_searches_minutes"
	keyMax      = "cache_max_results"
)

// Counters are hits and misses since the app started.
type Counters struct {
	Hits   int64     `json:"hits"`   // answered from the cache
	Misses int64     `json:"misses"` // fetched from the API
	Stale  int64     `json:"stale"`  // the API was down, so an expired copy was used
	Since  time.Time `json:"since"`  // when the app started (counters reset on restart)
}

// Client proxies requests to the Flixarr API through the cache.
type Client struct {
	Base  string
	HTTP  *http.Client
	Store *store.Store

	started time.Time
	mu      sync.RWMutex
	cfg     Config
	hits    atomic.Int64
	misses  atomic.Int64
	stale   atomic.Int64
	inserts atomic.Int64
}

// New returns an API client. base is e.g. https://api.flixarr.app.
func New(base, userAgent string, st *store.Store) *Client {
	c := &Client{Base: strings.TrimRight(base, "/"), Store: st, started: time.Now().UTC()}
	c.HTTP = &http.Client{Timeout: 15 * time.Second, Transport: uaTransport{ua: userAgent}}
	c.cfg = c.loadConfig()
	return c
}

type uaTransport struct{ ua string }

func (t uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("User-Agent", t.ua)
	return http.DefaultTransport.RoundTrip(r)
}

func (c *Client) loadConfig() Config {
	d := DefaultConfig
	num := func(key string, def int) int {
		if n, err := strconv.Atoi(c.Store.Setting(key, "")); err == nil && (n > 0 || (key == keyPages && n == Auto)) {
			return n
		}
		return def
	}
	return Config{
		Enabled:     c.Store.Setting(keyEnabled, "true") == "true",
		PagesMin:    num(keyPages, d.PagesMin),
		ListsMin:    num(keyLists, d.ListsMin),
		SearchesMin: num(keySearches, d.SearchesMin),
		MaxResults:  num(keyMax, d.MaxResults),
	}
}

// Config returns the current cache settings.
func (c *Client) Config() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg
}

// SetConfig validates, saves and applies new cache settings.
func (c *Client) SetConfig(cfg Config) error {
	times := []int{cfg.ListsMin, cfg.SearchesMin}
	if cfg.PagesMin != Auto {
		times = append(times, cfg.PagesMin)
	}
	for _, m := range times {
		if m < 5 || m > 30*24*60 {
			return errors.New("keep times must be between 5 minutes and 30 days")
		}
	}
	if cfg.MaxResults < 100 || cfg.MaxResults > 1000000 {
		return errors.New("the size limit must be between 100 and 1,000,000 results")
	}
	for k, v := range map[string]string{
		keyEnabled: strconv.FormatBool(cfg.Enabled), keyPages: strconv.Itoa(cfg.PagesMin), keyLists: strconv.Itoa(cfg.ListsMin),
		keySearches: strconv.Itoa(cfg.SearchesMin), keyMax: strconv.Itoa(cfg.MaxResults),
	} {
		if err := c.Store.SetSetting(k, v); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()
	if _, err := c.Store.CacheTrim(cfg.MaxResults); err != nil {
		return err
	}
	return nil
}

// Counters returns hits, misses and stale answers since the app started.
func (c *Client) Counters() Counters {
	return Counters{Hits: c.hits.Load(), Misses: c.misses.Load(), Stale: c.stale.Load(), Since: c.started}
}

// allowed paths under /v1 (anything else is rejected before leaving the server)
func allowed(p string) bool {
	for _, pre := range []string{"lists/", "movie/", "series/", "person/", "collection/", "search", "discover/", "genres/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

func kindOf(p string) string {
	switch {
	case strings.HasPrefix(p, "person/"):
		return KindPeople
	case strings.HasPrefix(p, "search"):
		return KindSearches
	case strings.HasPrefix(p, "lists/"), strings.HasPrefix(p, "discover/"), strings.HasPrefix(p, "genres/"):
		return KindLists
	}
	return KindTitles
}

// keep is how long to save an answer of this kind. fresh is the API's own time for it (0 if it sent none).
func (cfg Config) keep(kind string, fresh time.Duration) time.Duration {
	switch kind {
	case KindLists:
		return time.Duration(cfg.ListsMin) * time.Minute
	case KindSearches:
		return time.Duration(cfg.SearchesMin) * time.Minute
	}
	if cfg.PagesMin == Auto {
		if fresh > 0 {
			return fresh
		}
		return 24 * time.Hour
	}
	return time.Duration(cfg.PagesMin) * time.Minute
}

// Get fetches /v1/<path> with the given query (region and language are added by the caller).
// It returns the raw JSON body and the upstream status code.
func (c *Client) Get(ctx context.Context, path string, q url.Values) ([]byte, int, error) {
	path = strings.TrimPrefix(path, "/")
	if !allowed(path) || strings.Contains(path, "..") {
		return nil, http.StatusNotFound, nil
	}
	key := path
	if enc := q.Encode(); enc != "" {
		key += "?" + enc
	}
	cfg := c.Config()

	var saved *store.CacheEntry
	if cfg.Enabled {
		if e, err := c.Store.CacheGet(key); err == nil {
			if time.Now().Before(e.Expires) {
				c.hits.Add(1)
				return e.Body, e.Status, nil
			}
			saved = e
		}
		c.misses.Add(1)
	}

	body, status, fresh, err := c.fetch(ctx, c.Base+"/v1/"+key)
	if err != nil || status >= 500 || status == http.StatusTooManyRequests {
		// The API is down or throttling us: an expired copy beats an error page.
		if saved != nil && saved.Status == http.StatusOK {
			c.stale.Add(1)
			return saved.Body, saved.Status, nil
		}
		return body, status, err
	}
	if cfg.Enabled && (status == http.StatusOK || status == http.StatusNotFound) {
		kind := kindOf(path)
		keep := cfg.keep(kind, fresh)
		if status == http.StatusNotFound {
			keep = time.Minute
		}
		if err := c.Store.CachePut(key, store.CacheEntry{Kind: kind, Status: status, Body: body, Expires: time.Now().Add(keep)}); err == nil {
			if c.inserts.Add(1)%100 == 0 {
				c.Store.CacheTrim(cfg.MaxResults)
			}
		}
	}
	return body, status, nil
}

// fetch returns the body, the status and how long the API says the answer stays fresh (its s-maxage).
func (c *Client) fetch(ctx context.Context, u string) ([]byte, int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, 0, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("Flixarr API: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, 0, 0, err
	}
	return body, res.StatusCode, freshFor(res.Header.Get("Cache-Control")), nil
}

// freshFor reads s-maxage from a Cache-Control header.
func freshFor(cc string) time.Duration {
	for _, part := range strings.Split(cc, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "s-maxage="); ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return time.Duration(n) * time.Second
			}
		}
	}
	return 0
}

// Cleanup removes expired answers (keeping a day of grace for the API-down fallback) and enforces the size limit.
func (c *Client) Cleanup() (int64, error) {
	n, err := c.Store.CacheDeleteExpired(time.Now().Add(-24 * time.Hour))
	if err != nil {
		return n, err
	}
	t, err := c.Store.CacheTrim(c.Config().MaxResults)
	return n + t, err
}
