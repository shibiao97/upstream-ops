package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bejix/upstream-ops/backend/config"
	"github.com/bejix/upstream-ops/backend/storage"
	"github.com/robfig/cron/v3"
)

type UserSchedulerFactory func(userID uint, cfg config.SchedulerConfig) *Scheduler

type UserRunner struct {
	settings *storage.UserSchedulerSettings
	users    *storage.Users
	factory  UserSchedulerFactory
	log      *slog.Logger
	tick     time.Duration
	last     map[string]time.Time
	cancel   context.CancelFunc
}

func NewUserRunner(settings *storage.UserSchedulerSettings, users *storage.Users, factory UserSchedulerFactory, log *slog.Logger) *UserRunner {
	return &UserRunner{
		settings: settings,
		users:    users,
		factory:  factory,
		log:      log,
		tick:     time.Minute,
		last:     map[string]time.Time{},
	}
}

func (r *UserRunner) Start() {
	if r == nil || r.settings == nil || r.users == nil || r.factory == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() {
		t := time.NewTicker(r.tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				r.run(now)
			}
		}
	}()
}

func (r *UserRunner) Stop() {
	if r != nil && r.cancel != nil {
		r.cancel()
	}
}

func (r *UserRunner) run(now time.Time) {
	rows, err := r.settings.List()
	if err != nil {
		r.warn("list user scheduler settings", err)
		return
	}
	users, err := r.users.List("")
	if err != nil {
		r.warn("list users for scheduler", err)
		return
	}
	enabled := map[uint]bool{}
	for _, u := range users {
		if u.Enabled && u.Role == storage.UserRoleUser {
			enabled[u.ID] = true
		}
	}
	for _, row := range rows {
		if !enabled[row.UserID] {
			continue
		}
		var cfg config.SchedulerConfig
		if err := json.Unmarshal([]byte(row.ConfigJSON), &cfg); err != nil {
			r.warn("decode user scheduler", err)
			continue
		}
		s := r.factory(row.UserID, cfg)
		if due(r.last, row.UserID, "balance", cfg.BalanceCron, now) {
			s.RunBalanceNow()
		}
		if due(r.last, row.UserID, "rates", cfg.RateCron, now) {
			s.RunRatesNow()
		}
		if due(r.last, row.UserID, "retention", cfg.Retention.Cron, now) {
			s.RunRetentionNow()
		}
	}
}

func due(last map[string]time.Time, userID uint, job, spec string, now time.Time) bool {
	if spec == "" {
		return false
	}
	key := fmt.Sprintf("%d:%s", userID, job)
	prev, ok := last[key]
	if !ok {
		last[key] = now
		return false
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sched, err := parser.Parse(spec)
	if err != nil {
		return false
	}
	next := sched.Next(prev)
	if !next.After(now) {
		last[key] = now
		return true
	}
	return false
}

func (r *UserRunner) warn(msg string, err error) {
	if r.log != nil {
		r.log.Warn(msg, "err", err)
	}
}
