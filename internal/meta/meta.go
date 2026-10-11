// Package meta fetches movie and series data from the Flixarr data server (api.flixarr.app).
// Installs never talk to TMDB directly.
package meta

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client proxies requests to the data server with a small in-memory cache.
type Client struct {
	Base string
	HTTP *http.Client

	mu    sync.Mutex
	cache map[string]entry
}

type entry struct {
	body    []byte
	status  int
	expires time.Time
}

// New returns a data-server client. base is e.g. https://api.flixarr.app.
func New(base, userAgent string) *Client {
	c := &Client{Base: strings.TrimRight(base, "/"), cache: map[string]entry{}}
	c.HTTP = &http.Client{Timeout: 15 * time.Second, Transport: uaTransport{ua: userAgent}}
	return c
}

type uaTransport struct{ ua string }

func (t uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("User-Agent", t.ua)
	return http.DefaultTransport.RoundTrip(r)
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

// Get fetches /v1/<path> with the given query (region and language are added by the caller).
// It returns the raw JSON body and the upstream status code.
func (c *Client) Get(ctx context.Context, path string, q url.Values) ([]byte, int, error) {
	path = strings.TrimPrefix(path, "/")
	if !allowed(path) || strings.Contains(path, "..") {
		return nil, http.StatusNotFound, nil
	}
	u := c.Base + "/v1/" + path
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	c.mu.Lock()
	if e, ok := c.cache[u]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.body, e.status, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("data server: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, 0, err
	}
	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNotFound {
		ttl := 10 * time.Minute
		if res.StatusCode == http.StatusNotFound {
			ttl = time.Minute
		}
		c.mu.Lock()
		if len(c.cache) > 2000 { // simple bound: drop everything rather than grow forever
			c.cache = map[string]entry{}
		}
		c.cache[u] = entry{body: body, status: res.StatusCode, expires: time.Now().Add(ttl)}
		c.mu.Unlock()
	}
	return body, res.StatusCode, nil
}
