// Command flixarr runs the Flixarr server.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // so TZ (e.g. America/New_York) works in the slim image; job times follow it

	"github.com/Flixarr/flixarr/internal/arrstatus"
	"github.com/Flixarr/flixarr/internal/backup"
	"github.com/Flixarr/flixarr/internal/images"
	"github.com/Flixarr/flixarr/internal/jobs"
	"github.com/Flixarr/flixarr/internal/live"
	"github.com/Flixarr/flixarr/internal/meta"
	"github.com/Flixarr/flixarr/internal/notify"
	"github.com/Flixarr/flixarr/internal/plex"
	"github.com/Flixarr/flixarr/internal/plexlib"
	"github.com/Flixarr/flixarr/internal/profiles"
	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/server"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/update"
	"github.com/Flixarr/flixarr/internal/version"
	"github.com/Flixarr/flixarr/internal/watchlist"
	"github.com/Flixarr/flixarr/internal/webui"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	dataDir := env("FLIXARR_DATA", "/data")
	addr := ":" + env("FLIXARR_PORT", "7575")
	api := env("FLIXARR_API", "https://api.flixarr.app")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Error("data dir", "err", err)
		os.Exit(1)
	}
	box, err := secret.Load(dataDir)
	if err != nil {
		log.Error("secret key", "err", err)
		os.Exit(1)
	}
	if restored, err := backup.ApplyRestore(dataDir); err != nil {
		log.Error("restore", "err", err)
		os.Exit(1)
	} else if restored {
		log.Info("database restored from a backup")
	}
	st, err := store.Open(dataDir, box)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Stable per-install id that plex.tv uses to recognise this Flixarr.
	clientID := st.Setting("plex_client_id", "")
	if clientID == "" {
		b := make([]byte, 16)
		rand.Read(b)
		clientID = "flixarr-" + hex.EncodeToString(b)
		st.SetSetting("plex_client_id", clientID)
	}

	// Daily "is there a newer build?" check (also how installs are counted). Off with FLIXARR_UPDATE_CHECK=false.
	checker := &update.Checker{Store: st, API: api, ForceOff: os.Getenv("FLIXARR_UPDATE_CHECK") == "false", Log: log,
		HTTP: &http.Client{Timeout: 10 * time.Second}}
	runCtx, stopChecks := context.WithCancel(context.Background())
	defer stopChecks()
	go checker.Run(runCtx)

	mc := meta.New(api, "Flixarr/"+version.Full(), st)
	ic := images.New(dataDir, st)

	// Scheduled jobs (Settings → Jobs); schedules are editable there.
	runner := &jobs.Runner{Store: st, Log: log}
	runner.Add(&jobs.Job{ID: "cache-cleanup", Name: "Clear expired cache", Default: "0 3 * * *",
		About: "Removes saved results that expired more than a day ago and keeps the cache under its size limit.",
		Run: func(context.Context) (string, error) {
			n, err := mc.Cleanup()
			return plural(n, "result") + " removed", err
		}})
	runner.Add(&jobs.Job{ID: "image-cleanup", Name: "Image cache cleanup", Default: "0 5 * * 0",
		About: "Removes saved posters and backdrops nobody has viewed for a while (Settings → Cache).",
		Run: func(context.Context) (string, error) {
			n, err := ic.Cleanup()
			return plural(int64(n), "image") + " removed", err
		}})
	hub := live.New() // changes to open browsers as they happen
	st.OnAlert = func(audience string) { hub.Publish(live.Event{Type: "alerts", For: live.ForAudience(audience)}) }
	arrMon := &arrstatus.Monitor{Store: st, Log: log, OnRefresh: func() { hub.Publish(live.Event{Type: "services", For: live.Admins}) }}
	notifier := &notify.Notifier{Store: st, Log: log}
	reqs := &requests.Service{Store: st, Log: log, Notify: func(ev string, r store.Request) { notifier.Send(notifier.FromRequest(ev, r)) }}
	runner.Add(&jobs.Job{ID: "arr-status", Name: "Radarr / Sonarr: refresh status", Default: "* * * * *",
		About: "Reads each Radarr and Sonarr connection's queue, health, disk space, indexers and download clients for the Services cards. (Requests are followed separately, every few seconds.)",
		Run:   arrMon.Refresh})
	// Flixarr's quality profiles: downloaded from the API, added to Radarr/Sonarr on request, kept up to date.
	pm := &profiles.Manager{Store: st, API: api, UserAgent: "Flixarr/" + version.Full(), Log: log}
	runner.Add(&jobs.Job{ID: "profiles-sync", Name: "Quality profiles: sync", Default: "30 4 * * *",
		About: "Downloads Flixarr's quality profiles and updates the ones added to Radarr and Sonarr. Profiles edited there are left alone.",
		Run:   pm.SyncAll})
	// The daily update check also says which profiles version is current; a new one syncs right away.
	checker.OnResult = func(r *update.Result) {
		if r.Profiles != nil && (pm.Doc() == nil || pm.Doc().Version != r.Profiles.Version) {
			runner.RunNow(runCtx, "profiles-sync")
		}
	}
	// Plex: what's in the server's libraries, so titles show as available whoever added them.
	lib := &plexlib.Library{Store: st, ClientID: clientID, Version: version.Full(), Log: log}
	reqs.Plex = lib
	lib.OnAdded = func(keys []string) {
		hub.Publish(live.Event{Type: "plex", Data: keys})
		go reqs.FollowImporting(runCtx) // anything waiting for Plex: check now
	}
	go lib.Listen(runCtx) // Plex's own "added to the library" messages
	// Follow active requests: each is checked when it's due (every 30 s while downloading or importing,
	// every 2 minutes while waiting for a grab); webhooks and Plex messages check at once. Idle when nothing
	// is active: one small database read every 5 seconds.
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				ctx, cancel := context.WithTimeout(runCtx, 30*time.Second)
				if err := reqs.Refresh(ctx); err != nil {
					log.Warn("request follow", "err", err)
				}
				cancel()
			}
		}
	}()
	runner.Add(&jobs.Job{ID: "plex-full", Name: "Plex: full library scan", Default: "0 3 * * *",
		About: "Reads every title in the chosen Plex libraries, and notices ones that were removed.",
		Run:   lib.FullScan})
	runner.Add(&jobs.Job{ID: "plex-recent", Name: "Plex: recently added scan", Default: "*/30 * * * *",
		About: "Reads the newest additions to the chosen Plex libraries. A backup: Plex itself tells Flixarr about new titles the moment they're added.",
		Run:   lib.RecentScan})
	wl := &watchlist.Syncer{Store: st, Requests: reqs, Meta: mc, ClientID: clientID, Log: log}
	runner.Add(&jobs.Job{ID: "watchlist", Name: "Plex Watchlist: sync", Default: "*/15 * * * *",
		About: "Requests what people add to their Plex Watchlist, for those who switched it on in their Profile.",
		Run:   wl.Run})
	backups := &backup.Manager{Store: st, DataDir: dataDir}
	runner.Add(&jobs.Job{ID: "backup", Name: "Backup", Default: "0 2 * * *",
		About: "Saves a copy of Flixarr's database (settings, users, requests, issues) in the data folder's backups folder, keeping the newest few.",
		Run:   func(context.Context) (string, error) { return backups.Run() }})
	runner.Start(runCtx)
	runner.RunNow(runCtx, "profiles-sync")
	runner.RunNow(runCtx, "arr-status") // fill the cards right away instead of waiting for the first minute

	srv := &server.Server{
		Store:    st,
		Plex:     plex.New(clientID, version.Full()),
		Meta:     mc,
		Images:   ic,
		Jobs:     runner,
		Update:   checker,
		Arr:      arrMon,
		Profiles: pm,
		Requests: reqs,
		Library:  lib,
		Notify:   notifier,
		Backups:  backups,
		Live:     hub,
		Web:      webui.FS(),
		Log:      log,
		BaseCtx:  runCtx,
	}
	reqs.Changed = srv.PublishRequest // request changes reach open pages

	hs := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		log.Info("Flixarr starting", "version", version.Full(), "commit", version.Commit, "channel", version.Channel, "addr", addr, "data", dataDir)
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hs.Shutdown(ctx)
	log.Info("Flixarr stopped")
}

func plural(n int64, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.FormatInt(n, 10) + " " + word + "s"
}
