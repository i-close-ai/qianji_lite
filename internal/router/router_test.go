package router_test

import (
	"math/rand"
	"testing"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/router"
	"github.com/i-close-ai/qianji_lite/internal/state"
)

func testCfg() config.Config {
	cfg := config.Default()
	cfg.Strategy.StickyProbability = 1
	cfg.Ordinary.Routes = []config.Route{
		{ID: "heavy", Circuit: "p:heavy", Provider: "p", Model: "heavy", Weight: 90},
		{ID: "light", Circuit: "p:light", Provider: "p", Model: "light", Weight: 10},
	}
	return cfg
}

func TestWeightedRandomDistribution(t *testing.T) {
	cfg := testCfg()
	cfg.Strategy.StickyProbability = 0
	st := state.Default(cfg)
	rng := rand.New(rand.NewSource(1))
	counts := map[string]int{}
	n := 2000
	for i := 0; i < n; i++ {
		route := router.Select(cfg, &st, 0, "", nil, rng)
		counts[route.ID]++
	}
	if counts["heavy"] < 1600 || counts["heavy"] > 1950 {
		t.Fatalf("heavy=%d light=%d", counts["heavy"], counts["light"])
	}
}

func TestAffinitySticky(t *testing.T) {
	cfg := testCfg()
	st := state.Default(cfg)
	hash := router.HashAffinityKey("same request")
	router.RememberAffinity(&st, hash, "light", 100)
	rng := rand.New(rand.NewSource(2))
	route := router.Select(cfg, &st, 100, hash, nil, rng)
	if route == nil || route.ID != "light" || route.Via != "affinity" {
		t.Fatalf("%+v", route)
	}
}

func TestCircuitSkipsBlocked(t *testing.T) {
	cfg := testCfg()
	st := state.Default(cfg)
	delay := router.MarkFailure(cfg, &st, "heavy", "boom", 1000)
	if delay != 120 {
		t.Fatalf("delay=%d", delay)
	}
	rng := rand.New(rand.NewSource(3))
	route := router.Select(cfg, &st, 1000, "", nil, rng)
	if route.ID != "light" {
		t.Fatalf("got %s", route.ID)
	}
	router.MarkSuccess(cfg, &st, "heavy", 2000)
	if st.Circuits["p:heavy"].BlockedUntil != 0 {
		t.Fatal("circuit should clear")
	}
}

func TestMarkTimeoutDoesNotTripCircuit(t *testing.T) {
	cfg := testCfg()
	st := state.Default(cfg)
	router.MarkTimeout(cfg, &st, "heavy", 1000)
	if st.Routes["heavy"].Timeouts != 1 {
		t.Fatalf("timeouts=%d", st.Routes["heavy"].Timeouts)
	}
	if st.Circuits["p:heavy"].BlockedUntil != 0 {
		t.Fatal("timeout must not open the circuit")
	}
	if st.Circuits["p:heavy"].ConsecutiveFailures != 0 {
		t.Fatal("timeout must not count as provider failure")
	}
}

func TestSelectDoesNotRecordSelection(t *testing.T) {
	cfg := testCfg()
	st := state.Default(cfg)
	rng := rand.New(rand.NewSource(5))
	route := router.Select(cfg, &st, 0, "", nil, rng)
	if route == nil {
		t.Fatal("nil route")
	}
	if st.Routes[route.ID].Selections != 0 {
		t.Fatalf("select should not persist attempts, got %d", st.Routes[route.ID].Selections)
	}
	router.RecordSelection(&st, route.ID)
	if st.Routes[route.ID].Selections != 1 {
		t.Fatal("record should increment")
	}
}

func TestTimeoutDoesNotUseCircuitInSelectExclude(t *testing.T) {
	cfg := testCfg()
	st := state.Default(cfg)
	exclude := map[string]struct{}{"heavy": {}}
	rng := rand.New(rand.NewSource(4))
	route := router.Select(cfg, &st, 0, "", exclude, rng)
	if route.ID != "light" {
		t.Fatalf("got %s", route.ID)
	}
}
