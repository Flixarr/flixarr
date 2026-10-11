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

	"github.com/Flixarr/flixarr/internal/images"
	"github.com/Flixarr/flixarr/internal/jobs"
	"github.com/Flixarr/flixarr/internal/meta"
	"github.com/Flixarr/flixarr/internal/plex"
	"github.com/Flixarr/flixarr/internal/server"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/update"
	"github.com/Flixarr/flixarr/internal/version"
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
	st, err := store.Open(dataDir)
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
	runner.Start(runCtx)

	srv := &server.Server{
		Store:   st,
		Plex:    plex.New(clientID, version.Full()),
		Meta:    mc,
		Images:  ic,
		Jobs:    runner,
		Update:  checker,
		Web:     webui.FS(),
		Log:     log,
		BaseCtx: runCtx,
	}
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
