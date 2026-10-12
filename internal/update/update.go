// Package update asks the Flixarr data server once a day whether a newer build exists.
//
// The question carries exactly: a random install ID (generated once, used for nothing else),
// the version, build and update channel, the OS and CPU type, and the database type.
// Admins can turn it off in Profile, or with FLIXARR_UPDATE_CHECK=false; off means nothing is sent.
package update

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/version"
)

// Settings keys.
const (
	keyEnabled   = "update_check"      // "true" / "false" (default true)
	keyInstallID = "install_id"        // random UUID, only ever sent with update checks
	keyResult    = "update_result"     // last answer (JSON)
	keyChecked   = "update_checked_at" // RFC3339
)

// Latest describes the newest build on a channel.
type Latest struct {
	Version  string `json:"version"`
	Build    string `json:"build"`
	Commit   string `json:"commit"`
	Released string `json:"released"`
}

// Result is what the data server answered.
type Result struct {
	UpdateAvailable bool    `json:"updateAvailable"`
	Latest          *Latest `json:"latest"`
	// The quality profiles' current version, so installs know when to sync them.
	Profiles *struct {
		Version string            `json:"version"`
		Revs    map[string]string `json:"revs"`
	} `json:"profiles,omitempty"`
}

// Checker runs the daily check.
type Checker struct {
	Store    *store.Store
	API      string // e.g. https://api.flixarr.app
	ForceOff bool   // FLIXARR_UPDATE_CHECK=false
	Log      *slog.Logger
	HTTP     *http.Client
	// OnResult runs after each successful check (the quality profiles hook into it).
	OnResult func(*Result)
}

// Enabled reports whether checks run (env override wins).
func (c *Checker) Enabled() bool {
	return !c.ForceOff && c.Store.Setting(keyEnabled, "true") != "false"
}

// Locked reports whether the env variable has forced checks off (the toggle can't turn them on).
func (c *Checker) Locked() bool { return c.ForceOff }

// SetEnabled turns daily checks on or off.
func (c *Checker) SetEnabled(on bool) error {
	return c.Store.SetSetting(keyEnabled, fmt.Sprint(on))
}

// installID returns this install's random ID, creating it on first use.
func (c *Checker) installID() string {
	if id := c.Store.Setting(keyInstallID, ""); id != "" {
		return id
	}
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	c.Store.SetSetting(keyInstallID, id)
	return id
}

// Payload is exactly what gets sent, shown to admins next to the toggle.
func (c *Checker) Payload() map[string]string {
	return map[string]string{
		"install": c.installID(),
		"version": version.Version,
		"build":   version.Build,
		"channel": version.Channel,
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"db":      "sqlite",
	}
}

// Check asks the data server now and stores the answer.
func (c *Checker) Check(ctx context.Context) (*Result, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("update checks are turned off")
	}
	q := url.Values{}
	for k, v := range c.Payload() {
		q.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.API, "/")+"/v1/update?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Flixarr/"+version.Full())
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("data server answered %s", res.Status)
	}
	var r Result
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(r)
	c.Store.SetSetting(keyResult, string(raw))
	c.Store.SetSetting(keyChecked, time.Now().UTC().Format(time.RFC3339))
	if c.OnResult != nil {
		c.OnResult(&r)
	}
	return &r, nil
}

// Status is the last known answer, for /api/status.
type Status struct {
	Enabled   bool    `json:"enabled"`
	Locked    bool    `json:"locked"`
	Available bool    `json:"available"`
	Latest    *Latest `json:"latest"`
	CheckedAt string  `json:"checkedAt"`
}

// Last returns the stored result.
func (c *Checker) Last() Status {
	s := Status{Enabled: c.Enabled(), Locked: c.Locked(), CheckedAt: c.Store.Setting(keyChecked, "")}
	var r Result
	if json.Unmarshal([]byte(c.Store.Setting(keyResult, "")), &r) == nil {
		s.Available, s.Latest = r.UpdateAvailable, r.Latest
	}
	return s
}

// Run checks a few minutes after start, then every 24 hours. Failures are logged and skipped.
func (c *Checker) Run(ctx context.Context) {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	jitter, _ := rand.Int(rand.Reader, big.NewInt(120))
	wait := time.Minute + time.Duration(jitter.Int64())*time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 24 * time.Hour
		if !c.Enabled() {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		r, err := c.Check(cctx)
		cancel()
		if err != nil {
			c.Log.Warn("update check", "err", err)
		} else if r.UpdateAvailable && r.Latest != nil {
			c.Log.Info("update available", "latest", r.Latest.Version+"."+r.Latest.Build)
		}
	}
}
