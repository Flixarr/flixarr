package arr

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Ports Radarr and Sonarr use out of the box.
var Ports = map[string]int{"radarr": 7878, "sonarr": 8989}

// Found is a server Discover answered for.
type Found struct {
	URL string `json:"url"`
}

var titleRe = regexp.MustCompile(`(?i)<title>([^<]*)</title>`)

// Discover looks for Radarr or Sonarr on the usual spots: the Docker name ("radarr"), this machine,
// the Docker host, and the neighbors on this server's own networks, on the default port. It only
// finds the address; the API key still has to be entered.
func Discover(ctx context.Context, kind string) []Found {
	port, ok := Ports[kind]
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	hosts := candidates(kind)
	hc := &http.Client{Timeout: 1500 * time.Millisecond}
	var (
		mu    sync.Mutex
		found []Found
		wg    sync.WaitGroup
		slots = make(chan struct{}, 64)
	)
	for _, h := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			base := fmt.Sprintf("http://%s:%d", host, port)
			if probe(ctx, hc, base, Kinds[kind]) {
				mu.Lock()
				found = append(found, Found{URL: base})
				mu.Unlock()
			}
		}(h)
	}
	wg.Wait()

	// Named hosts first (they survive address changes), then in the order they were tried.
	rank := map[string]int{}
	for i, h := range hosts {
		rank[fmt.Sprintf("http://%s:%d", h, port)] = i
	}
	sort.Slice(found, func(a, b int) bool { return rank[found[a].URL] < rank[found[b].URL] })
	return found
}

// probe checks the unauthenticated ping endpoint, then the page title, which names the app.
func probe(ctx context.Context, hc *http.Client, base, app string) bool {
	get := func(path string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return nil, err
		}
		return hc.Do(req)
	}
	res, err := get("/ping")
	if err != nil {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "OK") {
		return false
	}
	res, err = get("/") // follows the redirect to the sign-in page when sign-in is on
	if err != nil {
		return false
	}
	page, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	res.Body.Close()
	m := titleRe.FindSubmatch(page)
	if m == nil {
		return false
	}
	return titleFits(string(m[1]), app)
}

// titleFits: the page title names the app, or (a renamed instance, whose title is its own name) names no
// other *arr app. It's only asked on the app's own default port, after an *arr-style /ping answered.
func titleFits(title, app string) bool {
	t := strings.ToLower(title)
	if strings.Contains(t, strings.ToLower(app)) {
		return true
	}
	for _, other := range []string{"radarr", "sonarr", "lidarr", "readarr", "prowlarr", "whisparr", "bazarr"} {
		if strings.Contains(t, other) {
			return false
		}
	}
	return true
}

// candidates lists the hosts to try, best guesses first.
func candidates(kind string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	add(kind)
	add("localhost")
	add("host.docker.internal")
	gw := defaultGateway()
	if gw != nil {
		add(gw.String())
	}
	// Everyone on the same /24 as this server's addresses and its gateway.
	var nets []net.IP
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip := ipn.IP.To4(); ip != nil && !ip.IsLoopback() {
					nets = append(nets, ip)
				}
			}
		}
	}
	if gw != nil {
		nets = append(nets, gw)
	}
	for _, ip := range nets {
		for i := 1; i < 255; i++ {
			add(fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], i))
		}
	}
	return out
}

// defaultGateway reads the default route on Linux (inside Docker that's the host).
func defaultGateway() net.IP {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) > 2 && fields[1] == "00000000" {
			b, err := hex.DecodeString(fields[2])
			if err != nil || len(b) != 4 {
				return nil
			}
			ip := make(net.IP, 4)
			binary.BigEndian.PutUint32(ip, binary.LittleEndian.Uint32(b))
			return ip
		}
	}
	return nil
}
