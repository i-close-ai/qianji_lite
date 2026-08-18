package router

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/state"
)

var BackoffSeconds = []int64{120, 300, 1200, 3600}

func HashAffinityKey(material string) string {
	sum := sha256.Sum256([]byte(material))
	return fmt.Sprintf("%x", sum)
}

func DefaultAffinityMaterial(prompt, workdir string) string {
	prefix := strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if len(prefix) > 2000 {
		prefix = prefix[:2000]
	}
	return workdir + "\n" + prefix
}

func ResolveAffinityHash(affinityKey, prompt, workdir string) string {
	if strings.TrimSpace(affinityKey) != "" {
		return HashAffinityKey(strings.TrimSpace(affinityKey))
	}
	if prompt == "" {
		return ""
	}
	return HashAffinityKey(DefaultAffinityMaterial(prompt, workdir))
}

func resolveNow(now int64) int64 {
	if now < 0 {
		return time.Now().Unix()
	}
	return now
}

func Eligible(cfg config.Config, st *state.State, now int64, exclude map[string]struct{}) []config.Route {
	now = resolveNow(now)
	var healthy []config.Route
	for _, route := range cfg.Ordinary.Routes {
		if route.Weight <= 0 {
			continue
		}
		if _, skip := exclude[route.ID]; skip {
			continue
		}
		c := st.Circuits[route.Circuit]
		if c.BlockedUntil > now {
			continue
		}
		healthy = append(healthy, route)
	}
	if len(healthy) > 0 {
		return healthy
	}
	if len(exclude) == 0 {
		return nil
	}
	var fallback []config.Route
	for _, route := range cfg.Ordinary.Routes {
		if route.Weight <= 0 {
			continue
		}
		c := st.Circuits[route.Circuit]
		if c.BlockedUntil > now {
			continue
		}
		fallback = append(fallback, route)
	}
	return fallback
}

func weightedChoice(candidates []config.Route, rng *rand.Rand) config.Route {
	total := 0
	for _, route := range candidates {
		total += route.Weight
	}
	n := rng.Intn(total)
	for _, route := range candidates {
		n -= route.Weight
		if n < 0 {
			return route
		}
	}
	return candidates[len(candidates)-1]
}

func finalize(st *state.State, route config.Route, via string) config.Route {
	route.Via = via
	c := st.Routes[route.ID]
	c.Selections++
	st.Routes[route.ID] = c
	return route
}

func Select(cfg config.Config, st *state.State, now int64, affinityHash string, exclude map[string]struct{}, rng *rand.Rand) *config.Route {
	now = resolveNow(now)
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	eligible := Eligible(cfg, st, now, exclude)
	if len(eligible) == 0 {
		return nil
	}
	stickyP := cfg.Strategy.StickyProbability
	if affinityHash != "" {
		entry, ok := st.Affinity[affinityHash]
		ttl := int64(cfg.Strategy.AffinityTTLSeconds)
		if ok {
			var sticky *config.Route
			for i := range eligible {
				if eligible[i].ID == entry.RouteID {
					sticky = &eligible[i]
					break
				}
			}
			if sticky != nil && entry.LastUsed+ttl >= now && rng.Float64() < stickyP {
				selected := finalize(st, *sticky, "affinity")
				return &selected
			}
		}
	}
	selected := finalize(st, weightedChoice(eligible, rng), "weighted_random")
	return &selected
}

func RememberAffinity(st *state.State, affinityHash, routeID string, now int64) {
	if affinityHash == "" {
		return
	}
	now = resolveNow(now)
	prev := st.Affinity[affinityHash]
	st.Affinity[affinityHash] = state.AffinityEntry{
		RouteID:  routeID,
		LastUsed: now,
		Hits:     prev.Hits + 1,
	}
}

func ClearAffinity(st *state.State, affinityHash string) {
	if affinityHash == "" {
		return
	}
	delete(st.Affinity, affinityHash)
}

func MarkSuccess(cfg config.Config, st *state.State, routeID string, now int64) {
	now = resolveNow(now)
	lookup := config.RouteByID(cfg)
	route := lookup[routeID]
	rs := st.Routes[routeID]
	rs.Successes++
	rs.LastSuccess = now
	st.Routes[routeID] = rs
	cs := st.Circuits[route.Circuit]
	cs.Successes++
	cs.ConsecutiveFailures = 0
	cs.BlockedUntil = 0
	cs.LastSuccess = now
	cs.LastError = ""
	st.Circuits[route.Circuit] = cs
}

func MarkFailure(cfg config.Config, st *state.State, routeID, errMsg string, now int64) int64 {
	now = resolveNow(now)
	lookup := config.RouteByID(cfg)
	route := lookup[routeID]
	cs := st.Circuits[route.Circuit]
	failures := cs.ConsecutiveFailures + 1
	idx := failures - 1
	if idx >= len(BackoffSeconds) {
		idx = len(BackoffSeconds) - 1
	}
	delay := BackoffSeconds[idx]
	if len(errMsg) > 1000 {
		errMsg = errMsg[len(errMsg)-1000:]
	}
	rs := st.Routes[routeID]
	rs.Failures++
	rs.LastFailure = now
	rs.LastError = errMsg
	st.Routes[routeID] = rs
	cs.Failures++
	cs.ConsecutiveFailures = failures
	cs.BlockedUntil = now + delay
	cs.LastFailure = now
	cs.LastError = errMsg
	st.Circuits[route.Circuit] = cs
	return delay
}

func EarliestBlocked(st *state.State) int64 {
	var earliest int64
	first := true
	for _, c := range st.Circuits {
		if first || c.BlockedUntil < earliest {
			earliest = c.BlockedUntil
			first = false
		}
	}
	return earliest
}

func FormatTime(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format(time.RFC3339)
}
