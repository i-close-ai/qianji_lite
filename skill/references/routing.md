# Routing algorithms considered for Qianji

Qianji ordinary pool uses **weighted random + cache affinity + circuit breaker**. Strong/strongest tiers are pinned, not load-balanced.

## Adopted

| Algorithm | Why |
|---|---|
| **Weighted random** | Each ordinary route has a configurable `weight`. Traffic follows weights in expectation, without the rigid 100-request lockstep of smooth WRR. |
| **Cache / session affinity** | Prompt-cache hits (OpenAI/Claude style) require the same provider+model seeing a similar prefix. If this task already succeeded on a route, reuse it with `sticky_probability` (default 0.85). |
| **Circuit breaker** | Fail `provider+model` for 2m / 5m / 20m / 1h. Timeouts do not trip the breaker. |
| **Epsilon exploration** | The remaining `1 - sticky_probability` draws a fresh weighted-random route so recovered models still get traffic and weights are not frozen by one sticky assignment. |

Affinity key (first match):

1. `--affinity-key` (prefer the user's original request text; router hashes it)
2. `workdir +` first 2k normalized chars of the prompt

Stale affinity older than `affinity_ttl_seconds` (7 days) is ignored. A provider failure or timeout on the sticky route clears it so the retry can move.

## Reviewed, not adopted now

| Algorithm | Verdict |
|---|---|
| Smooth weighted round-robin | Exact 70:30 over 100 calls, but poor cache locality. Replaced by weighted random. |
| Two-level provider then model split | Extra config surface. Model-level weights already encode provider mix. |
| Least connections | Needs accurate in-flight tracking across host/Pi processes. |
| Power of two choices | Useful under high concurrency; ordinary Qianji runs are usually one task at a time. |
| UCB / Thompson sampling | HTTP 200 ≠ good patch. Would optimize for uptime, not task quality. |
| Semantic routers (RouteLLM) | Extra model call and drift. User already names 强/最强/普通. |
| Cascade / FrugalGPT | Conflicts with ordinary-pool randomness; user can escalate by saying 强模型. |
| Consistent hashing alone | Good locality, but ignores weights unless weighted rendezvous hashing is added. |

Revisit P2C or latency-EWMA if many ordinary tasks run in parallel and p95 latency diverges across routes.
