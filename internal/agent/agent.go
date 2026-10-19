// Package agent orchestrates collection, SQLite persistence, and outbound transport.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/gemvc/infrasible-agent/internal/collector"
	"github.com/gemvc/infrasible-agent/internal/config"
	"github.com/gemvc/infrasible-agent/internal/storage"
	"github.com/gemvc/infrasible-agent/internal/transport"
)

// Agent is the daemon runtime wiring collector, store, and network workers.
type Agent struct {
	Config       config.Config
	Store        *storage.Store
	Collector    collector.Reader
	Log          *slog.Logger
	AgentVersion string
}

// New builds an Agent from config-selected families, injects matching cadence
// into the store, and returns a ready daemon. enabled_families is fixed for the
// process lifetime; config changes require a restart.
func New(c config.Config, s *storage.Store, l *slog.Logger, agentVersion string) (*Agent, error) {
	names := c.EnabledFamilies
	if names == nil {
		names = collector.DefaultFamilyNames()
	}
	reg, err := collector.NewFromNames(names)
	if err != nil {
		return nil, fmt.Errorf("collector families: %w", err)
	}
	s.SetCadenceProfile(reg.CadenceProfile())
	c.EnabledFamilies = reg.Names()
	return &Agent{Config: c, Store: s, Collector: reg, Log: l, AgentVersion: agentVersion}, nil
}

// enqueueLatest keeps only the newest frame in a bounded Live channel.
// When full, the oldest buffered frame is dropped so a blocked bridge cannot
// stall collection or SQLite writes.
func enqueueLatest(live chan collector.Frame, f collector.Frame) {
	select {
	case live <- f:
	default:
		select {
		case <-live:
		default:
		}
		select {
		case live <- f:
		default:
		}
	}
}

// Run starts Live and Persistence workers and the 1 Hz / finalize tickers until ctx ends.
func (a *Agent) Run(ctx context.Context) error {
	live := make(chan collector.Frame, 2)
	var workers sync.WaitGroup
	sender := transport.Sender{Servers: a.Config.MonitoringServers, Store: a.Store, Log: a.Log, AgentVersion: a.AgentVersion}
	workers.Add(2)
	go func() { defer workers.Done(); sender.Live(ctx, live) }()
	go func() { defer workers.Done(); sender.Persistence(ctx) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	roll := time.NewTicker(10 * time.Second)
	defer roll.Stop()
	a.Log.Info("agent running",
		"agent_id", a.Store.AgentID(),
		"pid", os.Getpid(),
		"enabled_families", a.Config.EnabledFamilies,
	)
	defer func() { close(live); workers.Wait() }()
	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-ticker.C:
			// Tick events are not re-created to cover missed intervals.
			f := a.Collector.Read(ctx, t)
			writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := a.Store.Save(writeCtx, f)
			cancel()
			if err != nil {
				a.Log.Error("STORAGE_DEGRADED: lost sample; no false durability", "error", err)
			}
			// Independent Live; a blocked bridge cannot block collector/SQLite.
			enqueueLatest(live, f)
		case <-roll.C:
			workCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			if e := a.Store.Finalize(workCtx, time.Now()); e != nil {
				a.Log.Error("aggregation degraded", "error", e)
			}
			cancel()
		}
	}
}
