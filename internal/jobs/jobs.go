package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// Job is one scheduled task.
type Job struct {
	ID      string
	Name    string
	About   string // one line shown in Settings → Jobs
	Default string // cron schedule a new install starts with
	Run     func(ctx context.Context) (string, error)
}

// Run is the outcome of a job's last run.
type Run struct {
	At      time.Time `json:"at"`
	Seconds float64   `json:"seconds"`
	Result  string    `json:"result"`
	Error   string    `json:"error,omitempty"`
}

// Info is a job as Settings → Jobs shows it.
type Info struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	About    string    `json:"about"`
	Schedule string    `json:"schedule"`
	Default  string    `json:"default"`
	Enabled  bool      `json:"enabled"`
	Running  bool      `json:"running"`
	Next     time.Time `json:"next"`
	Last     *Run      `json:"last"`
}

// Runner checks every minute which jobs are due.
type Runner struct {
	Store *store.Store
	Log   *slog.Logger

	mu      sync.Mutex
	jobs    []*Job
	running map[string]bool
}

// ErrUnknown is returned for a job id that doesn't exist.
var ErrUnknown = errors.New("unknown job")

// Add registers a job.
func (r *Runner) Add(j *Job) {
	if r.running == nil {
		r.running = map[string]bool{}
	}
	r.jobs = append(r.jobs, j)
}

func (r *Runner) find(id string) *Job {
	for _, j := range r.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (r *Runner) schedule(j *Job) string { return r.Store.Setting("job_"+j.ID+"_schedule", j.Default) }
func (r *Runner) enabled(j *Job) bool {
	return r.Store.Setting("job_"+j.ID+"_enabled", "true") == "true"
}

// List returns every job with its schedule, next run and last run.
func (r *Runner) List() []Info {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Info, 0, len(r.jobs))
	for _, j := range r.jobs {
		in := Info{ID: j.ID, Name: j.Name, About: j.About, Schedule: r.schedule(j), Default: j.Default, Enabled: r.enabled(j), Running: r.running[j.ID]}
		if s, err := Parse(in.Schedule); err == nil && in.Enabled {
			in.Next = s.Next(time.Now())
		}
		if raw := r.Store.Setting("job_"+j.ID+"_last", ""); raw != "" {
			var last Run
			if json.Unmarshal([]byte(raw), &last) == nil {
				in.Last = &last
			}
		}
		out = append(out, in)
	}
	return out
}

// Set changes a job's schedule and on/off state.
func (r *Runner) Set(id, schedule string, enabled bool) error {
	j := r.find(id)
	if j == nil {
		return ErrUnknown
	}
	if _, err := Parse(schedule); err != nil {
		return err
	}
	if err := r.Store.SetSetting("job_"+id+"_schedule", schedule); err != nil {
		return err
	}
	if enabled {
		return r.Store.SetSetting("job_"+id+"_enabled", "true")
	}
	return r.Store.SetSetting("job_"+id+"_enabled", "false")
}

// RunNow starts a job in the background (no-op if it's already running).
func (r *Runner) RunNow(ctx context.Context, id string) error {
	j := r.find(id)
	if j == nil {
		return ErrUnknown
	}
	go r.exec(ctx, j)
	return nil
}

func (r *Runner) exec(ctx context.Context, j *Job) {
	r.mu.Lock()
	if r.running[j.ID] {
		r.mu.Unlock()
		return
	}
	r.running[j.ID] = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, j.ID)
		r.mu.Unlock()
	}()

	start := time.Now()
	result, err := j.Run(ctx)
	last := Run{At: start.UTC(), Seconds: time.Since(start).Seconds(), Result: result}
	if err != nil {
		last.Error = err.Error()
		r.Log.Warn("job failed", "job", j.ID, "err", err)
	} else {
		r.Log.Info("job done", "job", j.ID, "result", result)
	}
	b, _ := json.Marshal(last)
	r.Store.SetSetting("job_"+j.ID+"_last", string(b))
}

// Start checks the schedules at the top of every minute until ctx ends.
func (r *Runner) Start(ctx context.Context) {
	go func() {
		for {
			now := time.Now()
			select {
			case <-ctx.Done():
				return
			case <-time.After(now.Truncate(time.Minute).Add(time.Minute).Sub(now)):
			}
			t := time.Now()
			for _, j := range r.jobs {
				if !r.enabled(j) {
					continue
				}
				if s, err := Parse(r.schedule(j)); err == nil && s.Matches(t) {
					go r.exec(ctx, j)
				}
			}
		}
	}()
}
