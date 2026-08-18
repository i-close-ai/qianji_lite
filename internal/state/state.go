package state

import (
	"encoding/json"
	"os"
	"syscall"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
)

const Version = 6

type Counters struct {
	Current             int    `json:"current"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	BlockedUntil        int64  `json:"blocked_until"`
	LastFailure         int64  `json:"last_failure"`
	LastError           string `json:"last_error"`
	LastSuccess         int64  `json:"last_success"`
	Selections          int    `json:"selections"`
	Successes           int    `json:"successes"`
	Failures            int    `json:"failures"`
}

type AffinityEntry struct {
	RouteID  string `json:"route_id"`
	LastUsed int64  `json:"last_used"`
	Hits     int    `json:"hits"`
}

type PiSync struct {
	SHA256    string `json:"sha256"`
	CheckedOn string `json:"checked_on"`
	Reason    string `json:"reason,omitempty"`
	Force     bool   `json:"force,omitempty"`
	Source    string `json:"source,omitempty"`
	Models    int    `json:"models,omitempty"`
}

type State struct {
	Version   int                      `json:"version"`
	Circuits  map[string]Counters      `json:"circuits"`
	Routes    map[string]Counters      `json:"routes"`
	Affinity  map[string]AffinityEntry `json:"affinity"`
	PiSync    PiSync                   `json:"pi_sync"`
	CreatedAt int64                    `json:"created_at"`
	UpdatedAt int64                    `json:"updated_at"`
}

func emptyCounters() Counters {
	return Counters{}
}

func Default(cfg config.Config) State {
	now := time.Now().Unix()
	st := State{
		Version:   Version,
		Circuits:  map[string]Counters{},
		Routes:    map[string]Counters{},
		Affinity:  map[string]AffinityEntry{},
		PiSync:    PiSync{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, id := range config.CircuitIDs(cfg) {
		st.Circuits[id] = emptyCounters()
	}
	for _, route := range cfg.Ordinary.Routes {
		st.Routes[route.ID] = emptyCounters()
	}
	return st
}

func Normalize(raw State, cfg config.Config) State {
	now := time.Now().Unix()
	if raw.Circuits == nil {
		raw.Circuits = map[string]Counters{}
	}
	if raw.Routes == nil {
		raw.Routes = map[string]Counters{}
	}
	if raw.Affinity == nil {
		raw.Affinity = map[string]AffinityEntry{}
	}
	raw.Version = Version
	validCircuits := map[string]struct{}{}
	for _, id := range config.CircuitIDs(cfg) {
		validCircuits[id] = struct{}{}
		if _, ok := raw.Circuits[id]; !ok {
			raw.Circuits[id] = emptyCounters()
		}
	}
	for id := range raw.Circuits {
		if _, ok := validCircuits[id]; !ok {
			delete(raw.Circuits, id)
		}
	}
	validIDs := map[string]struct{}{}
	for _, route := range cfg.Ordinary.Routes {
		validIDs[route.ID] = struct{}{}
		if _, ok := raw.Routes[route.ID]; !ok {
			raw.Routes[route.ID] = emptyCounters()
		}
	}
	for id := range raw.Routes {
		if _, ok := validIDs[id]; !ok {
			delete(raw.Routes, id)
		}
	}
	ttl := int64(cfg.Strategy.AffinityTTLSeconds)
	for key, entry := range raw.Affinity {
		if _, ok := validIDs[entry.RouteID]; !ok || entry.LastUsed+ttl < now {
			delete(raw.Affinity, key)
		}
	}
	raw.UpdatedAt = now
	return raw
}

func WithLock(cfg config.Config, write bool, fn func(*State) error) error {
	if err := os.MkdirAll(config.Home(), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(config.LockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	st := Default(cfg)
	if raw, err := os.ReadFile(config.StatePath()); err == nil {
		var parsed State
		if json.Unmarshal(raw, &parsed) == nil {
			st = parsed
		}
	}
	st = Normalize(st, cfg)
	if err := fn(&st); err != nil {
		return err
	}
	if !write {
		return nil
	}
	st.UpdatedAt = time.Now().Unix()
	return writeAtomic(st)
}

func writeAtomic(st State) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(config.Home(), "qianji-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, config.StatePath())
}
